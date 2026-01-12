package reset_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/stage/reset"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/testutil"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "stage", "reset", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Unstage all changes")
	})
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestRun_UnstageAll(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage SSM Parameter Store parameters
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value1"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value2"),
		StagedAt:  time.Now(),
	})

	// Stage Secrets Manager secrets
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret1", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value1"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (2 SSM Parameter Store, 1 Secrets Manager)")

	// Verify all unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config1")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config2")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "secret1")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_UnstageParamOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only SSM Parameter Store parameters
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (1 SSM Parameter Store, 0 Secrets Manager)")
}

func TestRun_UnstageSecretOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only Secrets Manager secrets
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (0 SSM Parameter Store, 1 Secrets Manager)")
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListEntriesErr = errors.New("mock store error")

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock store error")
}

func TestRun_UnstageTagsOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only tag changes (no entry changes)
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})
	_ = store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (1 SSM Parameter Store, 1 Secrets Manager)")

	// Verify all unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetTag(t.Context(), staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_UnstageEntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage entry change
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	// Stage tag change (different resource)
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/other", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	// Stage secret entry
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	// Stage secret tag
	_ = store.StageTag(t.Context(), staging.ServiceSecret, "other-secret", staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	// 1 entry + 1 tag = 2 for param, 1 entry + 1 tag = 2 for secret
	assert.Contains(t, buf.String(), "Unstaged all changes (2 SSM Parameter Store, 2 Secrets Manager)")

	// Verify all unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/other")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetTag(t.Context(), staging.ServiceSecret, "other-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}
