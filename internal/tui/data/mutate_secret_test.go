package data_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/tui/data"
)

// awsSecretCap returns the AWS secret capability from the neutral matrix. AWS
// Secrets Manager is the service that carries force-delete, recovery-window,
// and restore, so it exercises every secret-mutator branch.
func awsSecretCap(t *testing.T) capability.ServiceCapability {
	t.Helper()

	for _, pc := range capability.All() {
		if pc.Provider != string(provider.ProviderAWS) {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == "secret" {
				return sc
			}
		}
	}

	t.Fatal("aws secret capability not found")

	return capability.ServiceCapability{}
}

// existingSecretName is the one name existingSecretStore reports as present.
const existingSecretName = "app/EXISTS"

// existingSecretStore is a providermock that reports one existing secret, so
// edit/delete/tag see a resource; Get for any other name is not found.
func existingSecretStore(value string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, got string, _ provider.VersionRef) (*domain.Entry, error) {
			if got == existingSecretName {
				return &domain.Entry{Name: existingSecretName, Value: value, Type: domain.ValueTypeSecret}, nil
			}

			return nil, provider.ErrNotFound
		},
	}
}

// storeWithoutRestore wraps a provider.Store so its dynamic type satisfies
// provider.Store but NOT provider.Restorer — the restore-unsupported case that
// a bare *providermock.Store (which always implements Restorer) cannot express.
type storeWithoutRestore struct{ provider.Store }

// newSecretMutator builds a secret mutator over the given provider store and the
// temp staging store, using the AWS Secrets Manager strategy. It mirrors
// newParamMutator.
func newSecretMutator(t *testing.T, provStore provider.Store) (data.Mutator, store.ReadWriteOperator) {
	t.Helper()

	resolve, st := tempStagingStore(t)

	mut := data.NewSecretMutator(
		awsSecretCap(t),
		provStore,
		func(s provider.Store) staging.FullStrategy { return staging.NewAWSSecretStrategy(s) },
		resolve,
	)

	return mut, st
}

// TestSecretMutator_Capability pins the capability passthrough: the secret
// mutator surfaces the AWS Secrets Manager capability (force-delete, recovery
// window, restore) so a dialog can gate its controls.
//
//nolint:paralleltest // symmetrical with the other mutator tests
func TestSecretMutator_Capability(t *testing.T) {
	mut, _ := newSecretMutator(t, &providermock.Store{})

	svcCap := mut.Capability()
	assert.Equal(t, "secret", svcCap.Service)
	assert.True(t, svcCap.HasForceDelete, "AWS secret exposes force-delete")
	assert.True(t, svcCap.HasRecoveryWindow, "AWS secret exposes a recovery window")
	assert.True(t, svcCap.HasRestore, "AWS secret is restorable")
}

// TestSecretMutator_StagedRoundTrip stages a create, an edit, a delete, and a tag
// through the secret mutator and asserts each lands in the working staging store
// under the secret service.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestSecretMutator_StagedRoundTrip(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		// A brand-new name: Get is not found, so a staged CREATE is recorded.
		mut, st := newSecretMutator(t, &providermock.Store{
			GetFunc: func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
				return nil, provider.ErrNotFound
			},
		})

		_, err := mut.Create(ctx, data.StagedKey{Name: "app/NEW"}, "v1", "", "", true)
		require.NoError(t, err)

		entry, err := st.GetEntry(ctx, staging.ServiceSecret, staging.EntryKey{Name: "app/NEW"})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		require.NotNil(t, entry.Value)
		assert.Equal(t, "v1", *entry.Value)
		// Secrets carry no value-type axis, so nothing is staged.
		assert.Empty(t, entry.ValueType, "a staged secret create carries no value type")
	})

	t.Run("edit", func(t *testing.T) {
		mut, st := newSecretMutator(t, existingSecretStore("old"))

		out, err := mut.Update(ctx, data.StagedKey{Name: existingSecretName}, "new", "", "", true)
		require.NoError(t, err)
		assert.False(t, out.Skipped)

		entry, err := st.GetEntry(ctx, staging.ServiceSecret, staging.EntryKey{Name: existingSecretName})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationUpdate, entry.Operation)
	})

	t.Run("edit matching current value is skipped", func(t *testing.T) {
		mut, _ := newSecretMutator(t, existingSecretStore("same"))

		out, err := mut.Update(ctx, data.StagedKey{Name: existingSecretName}, "same", "", "", true)
		require.NoError(t, err)
		assert.True(t, out.Skipped, "editing to the live value stages nothing")
	})

	t.Run("delete", func(t *testing.T) {
		mut, st := newSecretMutator(t, existingSecretStore("v"))

		// AWS Secrets Manager requires a 7-30 day recovery window for a soft delete.
		_, err := mut.Delete(ctx, data.StagedKey{Name: existingSecretName}, false, 7, true)
		require.NoError(t, err)

		entry, err := st.GetEntry(ctx, staging.ServiceSecret, staging.EntryKey{Name: existingSecretName})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationDelete, entry.Operation)
	})

	t.Run("add tag", func(t *testing.T) {
		mut, st := newSecretMutator(t, existingSecretStore("v"))

		_, err := mut.AddTag(ctx, data.StagedKey{Name: existingSecretName}, "owner", "team", true)
		require.NoError(t, err)

		tagEntry, err := st.GetTag(ctx, staging.ServiceSecret, staging.EntryKey{Name: existingSecretName})
		require.NoError(t, err)
		assert.Equal(t, "team", tagEntry.Add["owner"])
	})

	t.Run("remove tag", func(t *testing.T) {
		mut, st := newSecretMutator(t, existingSecretStore("v"))

		_, err := mut.RemoveTag(ctx, data.StagedKey{Name: existingSecretName}, "owner", true)
		require.NoError(t, err)

		tagEntry, err := st.GetTag(ctx, staging.ServiceSecret, staging.EntryKey{Name: existingSecretName})
		require.NoError(t, err)
		assert.Contains(t, tagEntry.Remove, "owner", "a staged secret untag records the removed key")
	})
}

// TestSecretMutator_ImmediateRouting asserts an immediate secret write reaches
// the provider store directly (never the staging store), including the
// force-delete path that appends provider.ForceDelete.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestSecretMutator_ImmediateRouting(t *testing.T) {
	ctx := context.Background()

	t.Run("create hits the provider", func(t *testing.T) {
		var created bool

		mut, st := newSecretMutator(t, &providermock.Store{
			CreateFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
				created = true

				return domain.Version{ID: "1"}, nil
			},
		})

		_, err := mut.Create(ctx, data.StagedKey{Name: "app/NEW"}, "v", "", "", false)
		require.NoError(t, err)
		assert.True(t, created, "immediate secret create calls the provider directly")

		// Nothing was staged.
		_, err = st.GetEntry(ctx, staging.ServiceSecret, staging.EntryKey{Name: "app/NEW"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("update reads then puts", func(t *testing.T) {
		var put bool

		provStore := existingSecretStore("old")
		provStore.PutFunc = func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
			put = true

			return domain.Version{ID: "2"}, nil
		}

		mut, _ := newSecretMutator(t, provStore)

		_, err := mut.Update(ctx, data.StagedKey{Name: existingSecretName}, "new", "", "", false)
		require.NoError(t, err)
		assert.True(t, put, "immediate secret update calls Put on the provider")
	})

	t.Run("soft delete passes no options", func(t *testing.T) {
		var gotForce bool

		mut, _ := newSecretMutator(t, &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
				for _, o := range opts {
					if _, ok := o.(provider.ForceDelete); ok {
						gotForce = true
					}
				}

				return nil
			},
		})

		_, err := mut.Delete(ctx, data.StagedKey{Name: existingSecretName}, false, 0, false)
		require.NoError(t, err)
		assert.False(t, gotForce, "a soft delete carries no ForceDelete option")
	})

	t.Run("force delete appends provider.ForceDelete", func(t *testing.T) {
		var gotForce bool

		mut, _ := newSecretMutator(t, &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
				for _, o := range opts {
					if _, ok := o.(provider.ForceDelete); ok {
						gotForce = true
					}
				}

				return nil
			},
		})

		_, err := mut.Delete(ctx, data.StagedKey{Name: existingSecretName}, true, 0, false)
		require.NoError(t, err)
		assert.True(t, gotForce, "a force delete appends provider.ForceDelete")
	})

	t.Run("add tag hits the provider", func(t *testing.T) {
		var added map[string]string

		mut, _ := newSecretMutator(t, &providermock.Store{
			TagFunc: func(_ context.Context, _ string, add map[string]string) error {
				added = add

				return nil
			},
		})

		_, err := mut.AddTag(ctx, data.StagedKey{Name: existingSecretName}, "owner", "team", false)
		require.NoError(t, err)
		assert.Equal(t, "team", added["owner"], "immediate secret add-tag calls Tag on the provider")
	})

	t.Run("remove tag hits the provider", func(t *testing.T) {
		var removed []string

		mut, _ := newSecretMutator(t, &providermock.Store{
			UntagFunc: func(_ context.Context, _ string, keys []string) error {
				removed = keys

				return nil
			},
		})

		_, err := mut.RemoveTag(ctx, data.StagedKey{Name: existingSecretName}, "owner", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"owner"}, removed, "immediate secret remove-tag calls Untag on the provider")
	})
}

// TestSecretMutator_Restore covers both restore outcomes: a provider that
// implements provider.Restorer restores via the use case, while a store that
// does NOT implement it yields ErrRestoreUnsupported before any call.
//
//nolint:paralleltest // sets HOME / SUVE_STAGING_KEY via t.Setenv
func TestSecretMutator_Restore(t *testing.T) {
	ctx := context.Background()

	t.Run("restorer provider restores", func(t *testing.T) {
		var restored string

		mut, _ := newSecretMutator(t, &providermock.Store{
			RestoreFunc: func(_ context.Context, name string) error {
				restored = name

				return nil
			},
		})

		_, err := mut.Restore(ctx, existingSecretName)
		require.NoError(t, err)
		assert.Equal(t, existingSecretName, restored, "the restore reaches the provider Restorer")
	})

	t.Run("restore error is surfaced", func(t *testing.T) {
		sentinel := errors.New("restore exploded")

		mut, _ := newSecretMutator(t, &providermock.Store{
			RestoreFunc: func(context.Context, string) error { return sentinel },
		})

		_, err := mut.Restore(ctx, existingSecretName)
		require.ErrorIs(t, err, sentinel, "a restore failure is surfaced unchanged")
	})

	t.Run("non-restorer store is unsupported", func(t *testing.T) {
		// storeWithoutRestore does not implement provider.Restorer, so the
		// capability-gate fallback triggers.
		mut, _ := newSecretMutator(t, storeWithoutRestore{Store: &providermock.Store{}})

		_, err := mut.Restore(ctx, existingSecretName)
		require.ErrorIs(t, err, data.ErrRestoreUnsupported)
		assert.Equal(t, "restore is not supported by this provider", err.Error(), "stringError renders its message")
	})
}
