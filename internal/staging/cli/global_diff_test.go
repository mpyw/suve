// The all-service command tests share one namespace with their fixtures in
// global_test.go.
//declscope:namespace global

package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// globalDiffStoreReturning builds a provider.Store mock whose Get returns an entry with
// the given value and version id. The staging diff path only calls Get (via
// FetchCurrent / FetchCurrentTags), so that is all the mock needs to implement.
func globalDiffStoreReturning(value, versionID string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: value, Version: domain.Version{ID: versionID}}, nil
		},
	}
}

// globalDiffStoreGetError builds a provider.Store mock whose Get fails with a genuine
// provider.ErrNotFound, simulating a resource that no longer exists.
func globalDiffStoreGetError(msg string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, fmt.Errorf("%w: %s", provider.ErrNotFound, msg)
		},
	}
}

// globalDiffStoreGetTransientError builds a provider.Store mock whose Get fails with a
// non-not-found error (e.g. throttling, expired credentials, a network blip),
// which must NOT trigger auto-unstaging of staged work.
func globalDiffStoreGetTransientError(msg string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errors.New(msg)
		},
	}
}

// globalDiffStoreWithTags builds a provider.Store mock whose Get returns an entry
// carrying the given tags (used to drive FetchCurrentTags for tag diffs).
func globalDiffStoreWithTags(tags ...domain.Tag) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Tags: tags}, nil
		},
	}
}

func TestGlobalDiffCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalDiffCommand(globalAWSConfig()), nil, "--help")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Show diff of all staged changes")
	})

	t.Run("no arguments allowed", func(t *testing.T) {
		t.Parallel()

		_, _, err := runLeafCmd(t, stgcli.NewGlobalDiffCommand(globalAWSConfig()), nil, "extra-arg")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestGlobalDiff_NothingStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	var stdout, stderr bytes.Buffer

	// Empty per-service stores produce no output (the command action prints the
	// warning).
	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("", "1")), store),
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning("", "1")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}

func TestGlobalDiff_ParamOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("old-value", "1")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "(AWS)")
	assert.Contains(t, output, "(staged)")
}

func TestGlobalDiff_SecretOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-secret"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning("old-secret", "abc123def456")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-secret")
	assert.Contains(t, output, "+new-secret")
}

func TestGlobalDiff_BothServices(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-new"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	err = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-new"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("param-old", "1")), store),
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning("secret-old", "abc123def456")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "-param-old")
	assert.Contains(t, output, "+param-new")
	assert.Contains(t, output, "-secret-old")
	assert.Contains(t, output, "+secret-new")
}

func TestGlobalDiff_DeleteOperations(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	err = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("existing-value", "1")), store),
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning("existing-secret", "abc123def456")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for deletion)")
	assert.Contains(t, output, "-existing-value")
	assert.Contains(t, output, "-existing-secret")
}

func TestGlobalDiff_IdenticalValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("same-value", "1")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged /app/config: identical to AWS current")

	// Verify actually unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestGlobalDiff_ParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"key":"new"}`),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning(`{"key":"old"}`, "1")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestGlobalDiff_ParamUpdateAutoUnstageWhenDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("parameter not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "no longer exists")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestGlobalDiff_SecretUpdateAutoUnstageWhenDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("secret not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "no longer exists")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestGlobalDiff_SecretIdenticalValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning("same-value", "abc123def456")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged my-secret: identical to AWS current")

	// Verify actually unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestGlobalDiff_SecretParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"key":"new"}`),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning(`{"key":"old"}`, "abc123def456")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestGlobalDiff_SecretParseJSONMixed(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("not-json"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreReturning(`{"key":"old"}`, "abc123def456")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{ParseJSON: true})
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "--parse-json has no effect")
}

func TestGlobalDiff_ParamCreateOperation(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-param"}, staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("New parameter"),
		StagedAt:    time.Now(),
	})

	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-param"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "platform"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("parameter not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(not in AWS)")
	assert.Contains(t, output, "(staged for creation)")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "New parameter")
	// Tags are now staged separately and displayed in tag diff section
}

func TestGlobalDiff_SecretCreateOperation(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "new-secret"}, staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("secret-value"),
		Description: lo.ToPtr("New secret"),
		StagedAt:    time.Now(),
	})

	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "new-secret"}, staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("secret not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(not in AWS)")
	assert.Contains(t, output, "(staged for creation)")
	assert.Contains(t, output, "+secret-value")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "New secret")
	// Tags are now staged separately and displayed in tag diff section
}

func TestGlobalDiff_CreateWithParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(`{"key":"value","nested":{"a":1}}`),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("parameter not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for creation)")
	// JSON should be formatted (has newlines)
	assert.Contains(t, output, "\"key\":")
}

func TestGlobalDiff_DeleteAutoUnstageWhenAlreadyDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("parameter not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "already deleted")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

// TestGlobalDiff_KeptStagedOnTransientFetchError verifies that a non-not-found fetch
// error (throttling, expired credentials, a network blip) on a read-only
// `stage diff` does NOT discard staged deletes/updates (#321).
func TestGlobalDiff_KeptStagedOnTransientFetchError(t *testing.T) {
	t.Parallel()

	for _, op := range []staging.Operation{staging.OperationDelete, staging.OperationUpdate} {
		t.Run(string(op), func(t *testing.T) {
			t.Parallel()

			store := testutil.NewMockStore()

			entry := staging.Entry{Operation: op, StagedAt: time.Now()}
			if op == staging.OperationUpdate {
				entry.Value = lo.ToPtr("new-value")
			}

			require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, entry))

			var stdout, stderr bytes.Buffer

			r := &stgcli.GlobalDiffRunner{
				Services: []*stagingusecase.DiffUseCase{
					globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetTransientError("throttled")), store),
				},
				ProviderLabel: "AWS",
				Stdout:        &stdout,
				Stderr:        &stderr,
			}

			require.NoError(t, r.Run(t.Context(), stgcli.GlobalDiffOptions{}))

			// Surfaced as a warning, but NOT unstaged.
			assert.Contains(t, stderr.String(), "throttled")
			assert.NotContains(t, stderr.String(), "unstaged")

			_, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
			require.NoError(t, err, "entry must remain staged after a transient fetch error")
		})
	}
}

// TestGlobalDiff_DeleteEmptyRemoteNotUnstaged verifies a staged delete of a resource
// whose remote value is the empty string is not cancelled by the
// identical-value shortcut (#323).
func TestGlobalDiff_DeleteEmptyRemoteNotUnstaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/empty"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("", "1")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	require.NoError(t, r.Run(t.Context(), stgcli.GlobalDiffOptions{}))
	assert.NotContains(t, stderr.String(), "unstaged")

	// The staged deletion must survive `stage diff`.
	_, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/empty", Namespace: ""})
	require.NoError(t, err)
}

// TestGlobalDiff_ParseJSONReformatOnlyUpdateKeptStaged verifies that a staged update
// which only reformats JSON (same content, different key order) is NOT unstaged
// by `stage diff -j`: the auto-unstage decision is made on raw values, so
// staged work does not depend on a display flag (#324).
func TestGlobalDiff_ParseJSONReformatOnlyUpdateKeptStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"b":2,"a":1}`),
		StagedAt:  time.Now(),
	}))

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning(`{"a":1,"b":2}`, "1")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	require.NoError(t, r.Run(t.Context(), stgcli.GlobalDiffOptions{ParseJSON: true}))
	assert.NotContains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "only in JSON formatting")

	// The staged update must survive `stage diff -j`.
	_, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	require.NoError(t, err)
}

func TestGlobalDiff_SecretDeleteAutoUnstageWhenAlreadyDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("secret not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "already deleted")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestGlobalDiff_MetadataWithDescription(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation:   staging.OperationUpdate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("Updated config"),
		StagedAt:    time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("old-value", "1")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "Updated config")
}

func TestGlobalDiff_MetadataWithTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "platform"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("old-value", "1")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	// Entry diff should be displayed (value change)
	assert.Contains(t, output, "--- /app/config")
	assert.Contains(t, output, "+++ /app/config")
	// Tags are now staged separately and would be displayed in tag diff section
}

func TestGlobalDiff_TagOnlyDiff(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only tag changes (no entry change)
	err := store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("no remote")), store),
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("no remote")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "(staged tag changes)")
	assert.Contains(t, output, "+")
	assert.Contains(t, output, "env=prod")
	assert.Contains(t, output, "team=api")
}

func TestGlobalDiff_TagOnlyRemovalsDiff(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only tag removals (no additions)
	err := store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Remove:   maputil.NewSet("deprecated", "old-tag"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("no remote")), store),
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("no remote")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "deprecated")
	assert.Contains(t, output, "old-tag")
}

func TestGlobalDiff_SecretTagDiff(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag changes
	err := store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("no remote")), store),
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("no remote")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "+")
	assert.Contains(t, output, "env=staging")
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "deprecated")
}

func TestGlobalDiff_SecretCreateWithParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "new-secret"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(`{"key":"value","nested":{"a":1}}`),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services: []*stagingusecase.DiffUseCase{
			globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("secret not found")), store),
		},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for creation)")
	// JSON should be formatted (has newlines)
	assert.Contains(t, output, "\"key\":")
}

func TestGlobalDiff_BothEntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage entry change
	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	// Stage tag change (different resource)
	err = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/other"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreReturning("old-value", "1")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	// Entry diff
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	// Tag diff
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/other")
}

func TestGlobalDiff_ParamTagDiffWithValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage param tag removals
	err := store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Remove:   maputil.NewSet("deprecated", "old-tag"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	// Provider returns current tag values for the removal preview.
	paramStore := globalDiffStoreWithTags(
		domain.Tag{Key: "deprecated", Value: "true"},
		domain.Tag{Key: "old-tag", Value: "legacy-value"},
		domain.Tag{Key: "other", Value: "not-staged"},
	)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(paramStore), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "deprecated=true")
	assert.Contains(t, output, "old-tag=legacy-value")
}

func TestGlobalDiff_SecretTagDiffWithValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag removals
	err := store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	// Provider returns current tag values for the removal preview.
	secretStore := globalDiffStoreWithTags(domain.Tag{Key: "deprecated", Value: "yes"})

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffSecret(staging.NewAWSSecretStrategy(secretStore), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "deprecated=yes")
}

func TestGlobalDiff_ParamTagDiffAPIError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage param tag removals
	err := store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(globalDiffStoreGetError("API error")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	// Should still show the tag key, just without value
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "deprecated")
}

func TestGlobalDiff_SecretTagDiffAPIError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag removals
	err := store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Remove:   maputil.NewSet("old-tag"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffSecret(staging.NewAWSSecretStrategy(globalDiffStoreGetError("API error")), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	// Should still show the tag key, just without value
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "old-tag")
}

func TestGlobalDiff_TagDiffWithMissingValue(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage param tag removals
	err := store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Remove:   maputil.NewSet("has-value", "no-value"),
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	// Provider returns only some of the staged tags (no-value not present).
	paramStore := globalDiffStoreWithTags(domain.Tag{Key: "has-value", Value: "found"})

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{globalDiffParam(staging.NewAWSParamStrategy(paramStore), store)},
		ProviderLabel: "AWS",
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stgcli.GlobalDiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "has-value=found")
	// no-value should appear without value since it's not in AWS
	assert.Contains(t, output, "no-value")
	assert.NotContains(t, output, "no-value=")
}

func globalDiffParam(s staging.DiffStrategy, st store.ReadWriteOperator) *stagingusecase.DiffUseCase {
	return globalDiffUseCase(s, st)
}

func globalDiffSecret(s staging.DiffStrategy, st store.ReadWriteOperator) *stagingusecase.DiffUseCase {
	return globalDiffUseCase(s, st)
}

// globalDiffUseCase builds one service's DiffUseCase labelled with the AWS
// provider, as the command layer does.
func globalDiffUseCase(s staging.DiffStrategy, st store.ReadWriteOperator) *stagingusecase.DiffUseCase {
	return &stagingusecase.DiffUseCase{Strategy: s, Store: st, RemoteLabel: "AWS"}
}

// TestGlobalDiff_DiffsEntriesUnderTheirNamespace guards the per-namespace diff path: the
// SAME key staged under two namespaces must be diffed against the CURRENT value
// fetched under EACH entry's own namespace. The dev entry's strategy reports a
// different current value ("cur-b"), so if namespace threading were broken the
// dev entry would diff against the null-namespace current ("cur-a") and "cur-b"
// would never appear.
func TestGlobalDiff_DiffsEntriesUnderTheirNamespace(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := testutil.NewMockStore()
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("new-a"), StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k", Namespace: "dev"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("new-b"), StagedAt: time.Now(),
	}))

	// Current value differs per namespace, so a mis-routed fetch is observable.
	nsCurrent := map[string]string{"": "cur-a", "dev": "cur-b"}

	svc := &stagingusecase.DiffUseCase{
		Store:       st,
		RemoteLabel: "Azure",
		Strategy:    staging.NewAWSParamStrategy(globalDiffStoreReturning("cur-a", "1")),
		StrategyFor: func(ns string) (staging.DiffStrategy, error) {
			return staging.NewAWSParamStrategy(globalDiffStoreReturning(nsCurrent[ns], "1")), nil
		},
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalDiffRunner{
		Services:      []*stagingusecase.DiffUseCase{svc},
		ProviderLabel: "Azure",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	require.NoError(t, r.Run(ctx, stgcli.GlobalDiffOptions{}))

	out := buf.String()
	assert.Contains(t, out, "cur-a")
	assert.Contains(t, out, "new-a")
	assert.Contains(t, out, "cur-b", "the dev entry must be diffed against the value fetched under the dev namespace")
	assert.Contains(t, out, "new-b")
	assert.Contains(t, out, "[dev]", "the dev-namespaced entry must be labelled with its namespace")
}
