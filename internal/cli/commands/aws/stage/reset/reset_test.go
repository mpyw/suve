package reset_test

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

	"github.com/mpyw/suve/internal/cli/commands/aws/stage/reset"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()

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
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestRun_UnstageAll(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage SSM Parameter Store parameters
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config1"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value1"),
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config2"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value2"),
		StagedAt:  time.Now(),
	})

	// Stage Secrets Manager secrets
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "secret1"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value1"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (2 SSM Parameter Store, 1 Secrets Manager)")

	// Verify all unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config1", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config2", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "secret1", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_UnstageParamOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only SSM Parameter Store parameters
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (1 SSM Parameter Store, 0 Secrets Manager)")
}

func TestRun_UnstageSecretOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only Secrets Manager secrets
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
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
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock store error")
}

func TestRun_UnstageTagsOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only tag changes (no entry changes)
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	_ = store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (1 SSM Parameter Store, 1 Secrets Manager)")

	// Verify all unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_UnstageEntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage entry change
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	// Stage tag change (different resource)
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/other"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	// Stage secret entry
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	// Stage secret tag
	_ = store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "other-secret"}, staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	// 1 entry + 1 tag = 2 for param, 1 entry + 1 tag = 2 for secret
	assert.Contains(t, buf.String(), "Unstaged all changes (2 SSM Parameter Store, 2 Secrets Manager)")

	// Verify all unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/other"})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "other-secret"})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_ListTagsError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListTagsErr = errors.New("mock list tags error")

	// Stage an entry so we get past the entry listing
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock list tags error")
}

func TestRun_UnstageAllError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.UnstageAllErr = errors.New("mock unstage all error")

	// Stage an entry
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer

	r := &reset.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock unstage all error")
}

// awsServices returns the AWS service specs (param + secret) used by the
// provider-wide reset command.
func awsServices() []stgcli.GlobalServiceSpec {
	return []stgcli.GlobalServiceSpec{
		{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory},
		{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory},
	}
}

// notConfiguredResolver mimics an Azure scope resolver whose resource is not
// named (e.g. no --store-name), signalling the service should be skipped.
func notConfiguredResolver(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}

// TestRun_SkipUnconfiguredService verifies reset skips services whose scope is
// not configured (an unconfigured service can hold no staged state), so a
// provider with only one service connected reports no error rather than failing.
func TestRun_SkipUnconfiguredService(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	r := &reset.Runner{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: notConfiguredResolver},
			{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: notConfiguredResolver},
		},
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

// TestRun_ResolverErrorPropagates verifies a non-sentinel resolver error is not
// swallowed by the skip path.
func TestRun_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")

	var buf bytes.Buffer

	r := &reset.Runner{
		Services: []stgcli.GlobalServiceSpec{
			{
				Service:       staging.ServiceParam,
				ParserFactory: staging.AWSParamParserFactory,
				ScopeResolver: func(_ context.Context) (staging.ResolvedScope, error) {
					return staging.ResolvedScope{}, wantErr
				},
			},
		},
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.ErrorIs(t, err, wantErr)
}
