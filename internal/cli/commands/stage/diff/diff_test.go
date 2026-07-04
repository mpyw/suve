package diff_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	stagediff "github.com/mpyw/suve/internal/cli/commands/stage/diff"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// storeReturning builds a provider.Store mock whose Get returns an entry with
// the given value and version id. The staging diff path only calls Get (via
// FetchCurrent / FetchCurrentTags), so that is all the mock needs to implement.
func storeReturning(value, versionID string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: value, Version: domain.Version{ID: versionID}}, nil
		},
	}
}

// storeGetError builds a provider.Store mock whose Get fails, simulating a
// resource that no longer exists in the provider.
func storeGetError(msg string) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errors.New(msg)
		},
	}
}

// storeWithTags builds a provider.Store mock whose Get returns an entry
// carrying the given tags (used to drive FetchCurrentTags for tag diffs).
func storeWithTags(tags ...domain.Tag) *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Tags: tags}, nil
		},
	}
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()

		var buf bytes.Buffer

		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "stage", "diff", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show diff of all staged changes")
	})

	t.Run("no arguments allowed", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "stage", "diff", "extra-arg"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestRun_NothingStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}

func TestRun_ParamOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeReturning("old-value", "1")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "(AWS)")
	assert.Contains(t, output, "(staged)")
}

func TestRun_SecretOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-secret"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeReturning("old-secret", "abc123def456")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-secret")
	assert.Contains(t, output, "+new-secret")
}

func TestRun_BothServices(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-new"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-new"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy:  staging.NewParamStrategy(storeReturning("param-old", "1")),
		SecretStrategy: staging.NewSecretStrategy(storeReturning("secret-old", "abc123def456")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "-param-old")
	assert.Contains(t, output, "+param-new")
	assert.Contains(t, output, "-secret-old")
	assert.Contains(t, output, "+secret-new")
}

func TestRun_DeleteOperations(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy:  staging.NewParamStrategy(storeReturning("existing-value", "1")),
		SecretStrategy: staging.NewSecretStrategy(storeReturning("existing-secret", "abc123def456")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for deletion)")
	assert.Contains(t, output, "-existing-value")
	assert.Contains(t, output, "-existing-secret")
}

func TestRun_IdenticalValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeReturning("same-value", "1")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged /app/config: identical to AWS current")

	// Verify actually unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_ParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"key":"new"}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeReturning(`{"key":"old"}`, "1")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_ParamUpdateAutoUnstageWhenDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeGetError("parameter not found")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "no longer exists")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_SecretUpdateAutoUnstageWhenDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeGetError("secret not found")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "no longer exists")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_SecretIdenticalValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeReturning("same-value", "abc123def456")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged my-secret: identical to AWS current")

	// Verify actually unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_SecretParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"key":"new"}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeReturning(`{"key":"old"}`, "abc123def456")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_SecretParseJSONMixed(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("not-json"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeReturning(`{"key":"old"}`, "abc123def456")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "--parse-json has no effect")
}

func TestRun_ParamCreateOperation(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("New parameter"),
		StagedAt:    time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/new-param", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "platform"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeGetError("parameter not found")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(not in AWS)")
	assert.Contains(t, output, "(staged for creation)")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "New parameter")
	// Tags are now staged separately and displayed in tag diff section
}

func TestRun_SecretCreateOperation(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "new-secret", staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("secret-value"),
		Description: lo.ToPtr("New secret"),
		StagedAt:    time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceSecret, "new-secret", staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeGetError("secret not found")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(not in AWS)")
	assert.Contains(t, output, "(staged for creation)")
	assert.Contains(t, output, "+secret-value")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "New secret")
	// Tags are now staged separately and displayed in tag diff section
}

func TestRun_CreateWithParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(`{"key":"value","nested":{"a":1}}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeGetError("parameter not found")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for creation)")
	// JSON should be formatted (has newlines)
	assert.Contains(t, output, "\"key\":")
}

func TestRun_DeleteAutoUnstageWhenAlreadyDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeGetError("parameter not found")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "already deleted")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_SecretDeleteAutoUnstageWhenAlreadyDeleted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeGetError("secret not found")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "already deleted")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_MetadataWithDescription(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation:   staging.OperationUpdate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("Updated config"),
		StagedAt:    time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeReturning("old-value", "1")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "Updated config")
}

func TestRun_MetadataWithTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "platform"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeReturning("old-value", "1")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	// Entry diff should be displayed (value change)
	assert.Contains(t, output, "--- /app/config")
	assert.Contains(t, output, "+++ /app/config")
	// Tags are now staged separately and would be displayed in tag diff section
}

func TestRun_TagOnlyDiff(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only tag changes (no entry change)
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "(staged tag changes)")
	assert.Contains(t, output, "+")
	assert.Contains(t, output, "env=prod")
	assert.Contains(t, output, "team=api")
}

func TestRun_TagOnlyRemovalsDiff(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only tag removals (no additions)
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("deprecated", "old-tag"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "deprecated")
	assert.Contains(t, output, "old-tag")
}

func TestRun_SecretTagDiff(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag changes
	err := store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "+")
	assert.Contains(t, output, "env=staging")
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "deprecated")
}

func TestRun_SecretCreateWithParseJSON(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "new-secret", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(`{"key":"value","nested":{"a":1}}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeGetError("secret not found")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for creation)")
	// JSON should be formatted (has newlines)
	assert.Contains(t, output, "\"key\":")
}

func TestRun_BothEntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage entry change
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	// Stage tag change (different resource)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/other", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeReturning("old-value", "1")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
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

func TestRun_ParamTagDiffWithValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage param tag removals
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("deprecated", "old-tag"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	// Provider returns current tag values for the removal preview.
	paramStore := storeWithTags(
		domain.Tag{Key: "deprecated", Value: "true"},
		domain.Tag{Key: "old-tag", Value: "legacy-value"},
		domain.Tag{Key: "other", Value: "not-staged"},
	)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(paramStore),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "deprecated=true")
	assert.Contains(t, output, "old-tag=legacy-value")
}

func TestRun_SecretTagDiffWithValues(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag removals
	err := store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	// Provider returns current tag values for the removal preview.
	secretStore := storeWithTags(domain.Tag{Key: "deprecated", Value: "yes"})

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(secretStore),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "deprecated=yes")
}

func TestRun_ParamTagDiffAPIError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage param tag removals
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(storeGetError("API error")),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	// Should still show the tag key, just without value
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "deprecated")
}

func TestRun_SecretTagDiffAPIError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag removals
	err := store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Remove:   maputil.NewSet("old-tag"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		SecretStrategy: staging.NewSecretStrategy(storeGetError("API error")),
		Store:          store,
		Stdout:         &stdout,
		Stderr:         &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	// Should still show the tag key, just without value
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "old-tag")
}

func TestRun_TagDiffWithMissingValue(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage param tag removals
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("has-value", "no-value"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	// Provider returns only some of the staged tags (no-value not present).
	paramStore := storeWithTags(domain.Tag{Key: "has-value", Value: "found"})

	var stdout, stderr bytes.Buffer

	r := &stagediff.Runner{
		ParamStrategy: staging.NewParamStrategy(paramStore),
		Store:         store,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "has-value=found")
	// no-value should appear without value since it's not in AWS
	assert.Contains(t, output, "no-value")
	assert.NotContains(t, output, "no-value=")
}
