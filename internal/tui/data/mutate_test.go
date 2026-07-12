package data_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/tui/data"
)

// awsParamCap returns the AWS param capability from the neutral matrix.
func awsParamCap(t *testing.T) capability.ServiceCapability {
	t.Helper()

	for _, pc := range capability.All() {
		if pc.Provider != string(provider.ProviderAWS) {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == "param" {
				return sc
			}
		}
	}

	t.Fatal("aws param capability not found")

	return capability.ServiceCapability{}
}

// tempStagingStore configures a temp-home, env-keyed working store (the CI
// SUVE_STAGING_KEY discipline), cached so a single scope never races the
// keychain, and returns the resolver plus the store for read-back assertions.
func tempStagingStore(t *testing.T) (data.StagingStoreResolver, store.ReadWriteOperator) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	// The all-zero base64 key CI exports; encrypts without touching the keychain.
	t.Setenv("SUVE_STAGING_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	var (
		once   sync.Once
		cached store.ReadWriteOperator
		cErr   error
	)

	resolve := func() (store.ReadWriteOperator, error) {
		once.Do(func() { cached, cErr = file.NewWorkingStore(scope) })

		return cached, cErr
	}

	st, err := resolve()
	require.NoError(t, err)

	return resolve, st
}

// existingParamName is the one name existingParamStore reports as present.
const existingParamName = "/app/EXISTS"

// existingParamStore is a providermock that reports one existing parameter, so
// edit/delete/tag see a resource; Get for any other name is not found.
func existingParamStore(value string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, got string, _ provider.VersionRef) (*domain.Entry, error) {
			if got == existingParamName {
				return &domain.Entry{Name: existingParamName, Value: value, Type: domain.ValueTypePlaintext}, nil
			}

			return nil, provider.ErrNotFound
		},
	}
}

// newParamMutator builds a param mutator over the given provider store and the
// temp staging store, using the AWS SSM strategy.
func newParamMutator(t *testing.T, provStore provider.Store) (data.Mutator, store.ReadWriteOperator) {
	t.Helper()

	resolve, st := tempStagingStore(t)

	mut := data.NewParamMutator(
		awsParamCap(t),
		func(context.Context, string) (provider.Store, error) { return provStore, nil },
		func(s provider.Store) staging.FullStrategy { return staging.NewAWSParamStrategy(s) },
		resolve,
	)

	return mut, st
}

// TestParamMutator_StagedRoundTrip stages a create, an edit, a delete, and a tag
// through the mutator and asserts each lands in the working staging store.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestParamMutator_StagedRoundTrip(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		// A brand-new name: Get is not found, so a staged CREATE is recorded.
		mut, st := newParamMutator(t, &providermock.Store{
			GetFunc: func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
				return nil, provider.ErrNotFound
			},
		})

		_, err := mut.Create(ctx, data.StagedKey{Name: "/app/NEW"}, "v1", "String", "", true)
		require.NoError(t, err)

		entry, err := st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/NEW"})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		require.NotNil(t, entry.Value)
		assert.Equal(t, "v1", *entry.Value)
	})

	t.Run("edit", func(t *testing.T) {
		mut, st := newParamMutator(t, existingParamStore("old"))

		out, err := mut.Update(ctx, data.StagedKey{Name: "/app/EXISTS"}, "new", "String", "", true)
		require.NoError(t, err)
		assert.False(t, out.Skipped)

		entry, err := st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/EXISTS"})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationUpdate, entry.Operation)
	})

	t.Run("edit matching current value is skipped", func(t *testing.T) {
		mut, _ := newParamMutator(t, existingParamStore("same"))

		out, err := mut.Update(ctx, data.StagedKey{Name: "/app/EXISTS"}, "same", "String", "", true)
		require.NoError(t, err)
		assert.True(t, out.Skipped, "editing to the live value stages nothing")
	})

	t.Run("delete", func(t *testing.T) {
		mut, st := newParamMutator(t, existingParamStore("v"))

		_, err := mut.Delete(ctx, data.StagedKey{Name: "/app/EXISTS"}, false, 0, true)
		require.NoError(t, err)

		entry, err := st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/EXISTS"})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationDelete, entry.Operation)
	})

	t.Run("tag", func(t *testing.T) {
		mut, st := newParamMutator(t, existingParamStore("v"))

		_, err := mut.AddTag(ctx, data.StagedKey{Name: "/app/EXISTS"}, "owner", "team", true)
		require.NoError(t, err)

		tagEntry, err := st.GetTag(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/EXISTS"})
		require.NoError(t, err)
		assert.Equal(t, "team", tagEntry.Add["owner"])
	})
}

// TestParamMutator_ImmediateRouting asserts an immediate write reaches the
// provider store directly and never touches the staging store.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestParamMutator_ImmediateRouting(t *testing.T) {
	ctx := context.Background()

	var created, deleted bool

	provStore := &providermock.Store{
		CreateFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
			created = true

			return domain.Version{ID: "1"}, nil
		},
		DeleteFunc: func(context.Context, string, ...provider.DeleteOption) error {
			deleted = true

			return nil
		},
	}

	mut, st := newParamMutator(t, provStore)

	_, err := mut.Create(ctx, data.StagedKey{Name: "/app/NEW"}, "v", "String", "", false)
	require.NoError(t, err)
	assert.True(t, created, "immediate create calls the provider directly")

	_, err = mut.Delete(ctx, data.StagedKey{Name: "/app/NEW"}, false, 0, false)
	require.NoError(t, err)
	assert.True(t, deleted, "immediate delete calls the provider directly")

	// Nothing was staged.
	_, err = st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/NEW"})
	require.ErrorIs(t, err, staging.ErrNotStaged)
}
