package data_test

import (
	"context"
	"errors"
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

// azureParamCap returns the Azure App Configuration param capability (the
// namespaced param service) from the neutral matrix.
func azureParamCap(t *testing.T) capability.ServiceCapability {
	t.Helper()

	for _, pc := range capability.All() {
		if pc.Provider != string(provider.ProviderAzure) {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == "param" {
				return sc
			}
		}
	}

	t.Fatal("azure param capability not found")

	return capability.ServiceCapability{}
}

// TestParamMutator_RejectsFilterNamespace pins the server-side namespace guard on
// the write path: a write whose App Configuration namespace names all/multiple
// namespaces (`*` or a `,` OR-list) is rejected by literalNamespace ->
// aznamespace.Literal BEFORE any provider or staging store is resolved, so a
// filter value can never be written as if it were one literal namespace.
//
//nolint:paralleltest // symmetrical with the other mutator tests; no shared state anyway
func TestParamMutator_RejectsFilterNamespace(t *testing.T) {
	ctx := context.Background()

	resolveCalled := false
	mut := data.NewParamMutator(
		azureParamCap(t),
		func(context.Context, string) (provider.Store, error) {
			resolveCalled = true

			return nil, errors.New("store must not be resolved for an invalid namespace")
		},
		func(provider.Store) staging.FullStrategy { return nil },
		func() (store.ReadWriteOperator, error) {
			return nil, errors.New("staging store must not be resolved for an invalid namespace")
		},
	)

	_, err := mut.Create(ctx, data.StagedKey{Name: "k", Namespace: "*"}, "v", "", "", true)
	require.Error(t, err, "a staged create into * is rejected")
	assert.Contains(t, err.Error(), "all/multiple namespaces")

	_, err = mut.Delete(ctx, data.StagedKey{Name: "k", Namespace: "dev,prod"}, false, 0, false)
	require.Error(t, err, "an immediate delete across an OR-list is rejected")
	assert.Contains(t, err.Error(), "all/multiple namespaces")

	assert.False(t, resolveCalled, "the namespace guard rejects before any store is resolved")
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

// TestParamMutator_StagedCarriesValueType pins the #664/#680 end-to-end fix: a
// staged SecureString create routed through the TUI param mutator stores the
// SecureString value type in the working staging store AND, on apply through the
// AWS SSM param strategy, writes the parameter as SecureString (domain
// ValueTypeSecret) rather than silently downgrading it to plaintext String.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestParamMutator_StagedCarriesValueType(t *testing.T) {
	ctx := context.Background()

	var (
		appliedType  domain.ValueType
		appliedValue string
	)

	provStore := &providermock.Store{
		GetFunc: func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
		CreateFunc: func(
			_ context.Context, _, value string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			appliedValue, appliedType = value, valueType

			return domain.Version{ID: "1"}, nil
		},
	}

	mut, st := newParamMutator(t, provStore)

	// TUI staged create with Type = SecureString.
	_, err := mut.Create(ctx, data.StagedKey{Name: "/app/SECRET"}, "s3cr3t", "SecureString", "", true)
	require.NoError(t, err)

	// The staged entry carries the SecureString value type end-to-end.
	entry, err := st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/SECRET"})
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, domain.ValueTypeSecret, entry.ValueType, "staged create carries the SecureString type")

	// Apply the staged create through the AWS SSM param strategy: the parameter is
	// written as SecureString, not the old hardcoded plaintext String.
	strategy := staging.NewAWSParamStrategy(provStore)
	require.NoError(t, strategy.Apply(ctx, "/app/SECRET", *entry))
	assert.Equal(t, "s3cr3t", appliedValue)
	assert.Equal(t, domain.ValueTypeSecret, appliedType, "apply writes the parameter as SecureString")
}

// TestParamMutator_StagedEditValueType pins the staged-edit value-type rules: an
// explicit type is stored (so a staged edit can change the type), while an empty
// type — passed by the staging-review edit, which cannot seed the current type —
// preserves the existing staged type instead of downgrading it.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestParamMutator_StagedEditValueType(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit type is stored", func(t *testing.T) {
		mut, st := newParamMutator(t, existingParamStore("old"))

		_, err := mut.Update(ctx, data.StagedKey{Name: existingParamName}, "new", "SecureString", "", true)
		require.NoError(t, err)

		entry, err := st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: existingParamName})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationUpdate, entry.Operation)
		assert.Equal(t, domain.ValueTypeSecret, entry.ValueType, "an explicit staged-edit type is stored")
	})

	t.Run("empty type preserves a staged SecureString create", func(t *testing.T) {
		mut, st := newParamMutator(t, &providermock.Store{
			GetFunc: func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
				return nil, provider.ErrNotFound
			},
		})

		// Stage a SecureString create, then edit its value with no type (the
		// staging-review edit path passes an empty type): the create's SecureString
		// type must survive rather than downgrade to plaintext.
		_, err := mut.Create(ctx, data.StagedKey{Name: "/app/NEW"}, "v1", "SecureString", "", true)
		require.NoError(t, err)

		_, err = mut.Update(ctx, data.StagedKey{Name: "/app/NEW"}, "v2", "", "", true)
		require.NoError(t, err)

		entry, err := st.GetEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "/app/NEW"})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation, "editing a staged create keeps it a create")
		require.NotNil(t, entry.Value)
		assert.Equal(t, "v2", *entry.Value, "the edit updates the draft value")
		assert.Equal(t, domain.ValueTypeSecret, entry.ValueType, "an empty edit type preserves the staged SecureString")
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

// TestParamMutator_ImmediateCreateUpserts pins the #691 fix: an immediate param
// create is a create-or-update (upsert), matching the GUI (ParamSet) and the CLI
// (`param set`). Creating over an EXISTING param no longer surfaces the raw
// provider.ErrAlreadyExists — it falls back to update and reports the outcome as
// an update so the dialog voices it. A create error that is NOT already-exists is
// still surfaced unchanged (never silently swallowed as an upsert).
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv (newParamMutator)
func TestParamMutator_ImmediateCreateUpserts(t *testing.T) {
	ctx := context.Background()

	t.Run("create over an existing param falls back to update", func(t *testing.T) {
		var createTried, put bool

		provStore := &providermock.Store{
			CreateFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
				createTried = true

				return domain.Version{}, provider.ErrAlreadyExists
			},
			// UpdateUseCase reads the entry before writing; report it as present.
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Name: existingParamName, Value: "old", Type: domain.ValueTypePlaintext}, nil
			},
			PutFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
				put = true

				return domain.Version{ID: "2"}, nil
			},
		}

		mut, _ := newParamMutator(t, provStore)

		out, err := mut.Create(ctx, data.StagedKey{Name: existingParamName}, "new", "String", "", false)
		require.NoError(t, err, "immediate create over an existing param upserts instead of raising already-exists")
		assert.True(t, createTried, "create is attempted first")
		assert.True(t, put, "the already-exists create falls back to update (Put)")
		assert.True(t, out.Updated, "the outcome reports an update so the dialog voices it")
	})

	t.Run("a non-already-exists create error is surfaced unchanged", func(t *testing.T) {
		sentinel := errors.New("provider exploded")

		provStore := &providermock.Store{
			CreateFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
				return domain.Version{}, sentinel
			},
		}

		mut, _ := newParamMutator(t, provStore)

		_, err := mut.Create(ctx, data.StagedKey{Name: "/app/NEW"}, "v", "String", "", false)
		require.ErrorIs(t, err, sentinel, "a non-already-exists error is returned, not treated as an upsert")
	})
}
