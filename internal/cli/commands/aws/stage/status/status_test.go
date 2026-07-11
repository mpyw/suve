package status_test

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

	"github.com/mpyw/suve/internal/cli/commands/aws/stage/status"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

func TestCommand_NoStagedChanges(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestCommand_ShowParamChangesOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value1"),
		StagedAt:  now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes")
	assert.Contains(t, output, "/app/config")
	assert.NotContains(t, output, "Staged Secrets Manager changes")
}

func TestCommand_ShowSecretChangesOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged Secrets Manager changes")
	assert.Contains(t, output, "my-secret")
	assert.NotContains(t, output, "Staged SSM Parameter Store changes")
}

func TestCommand_ShowBothParamAndSecretChanges(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  now,
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "M")
	assert.Contains(t, output, "Staged Secrets Manager changes")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "D")
}

func TestCommand_VerboseOutput(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test-value"),
		StagedAt:  now,
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged:")
	assert.Contains(t, output, "Value:")
	assert.Contains(t, output, "test-value")
	assert.Contains(t, output, "secret-value")
}

func TestCommand_VerboseWithDelete(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "Staged:")
	assert.NotContains(t, output, "Value:")
}

func TestCommand_VerboseTruncatesLongValue(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	longValue := "this is a very long value that exceeds one hundred characters and should be truncated in verbose mode output display"
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(longValue),
		StagedAt:  now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "...")
	assert.NotContains(t, output, "display")
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	app := apptest.AWSApp()

	var buf bytes.Buffer

	app.Writer = &buf

	// Test that the command exists and works. `status` is a subcommand of
	// `stage`, not a top-level command, so it must be invoked as `stage status`.
	err := app.Run(t.Context(), []string{"suve", "stage", "status", "--help"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "staged changes")
}

func TestCommand_StoreError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListEntriesErr = errors.New("mock store error")

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock store error")
}

func TestCommand_ShowParamTagChangesOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		StagedAt: now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "T")
	assert.Contains(t, output, "+2 tag(s)")
	assert.NotContains(t, output, "Staged Secrets Manager changes")
}

func TestCommand_ShowSecretTagChangesOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged Secrets Manager changes")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "T")
	assert.Contains(t, output, "+1 tag(s)")
	assert.Contains(t, output, "-1 tag(s)")
	assert.NotContains(t, output, "Staged SSM Parameter Store changes")
}

func TestCommand_ShowMixedEntryAndTagChanges(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	// Entry change
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  now,
	})

	// Tag change (different resource)
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/other"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes (2)")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "M")
	assert.Contains(t, output, "/app/other")
	assert.Contains(t, output, "T")
}

func TestCommand_TagChangesVerbose(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		Remove:   maputil.NewSet("deprecated", "old"),
		StagedAt: now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "T")
	assert.Contains(t, output, "/app/config")
	// Verbose output should show individual tags
	assert.Contains(t, output, "+ env=prod")
	assert.Contains(t, output, "+ team=api")
	assert.Contains(t, output, "- deprecated")
	assert.Contains(t, output, "- old")
}

func TestCommand_TagOnlyChangesNoEntries(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	now := time.Now()
	// Only tag changes, no entry changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/param"}, staging.TagEntry{
		Add:      map[string]string{"key": "value"},
		StagedAt: now,
	})

	_ = store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Remove:   maputil.NewSet("old-tag"),
		StagedAt: now,
	})

	var buf bytes.Buffer

	r := &status.Runner{
		Store:    store,
		Services: awsServices(),
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes (1)")
	assert.Contains(t, output, "/app/param")
	assert.Contains(t, output, "Staged Secrets Manager changes (1)")
	assert.Contains(t, output, "my-secret")
	assert.NotContains(t, output, "No changes staged")
}

// awsServices returns the AWS service specs (param + secret) used by the
// provider-wide status command.
func awsServices() []stgcli.GlobalServiceSpec {
	return []stgcli.GlobalServiceSpec{
		{Service: staging.ServiceParam, ParserFactory: staging.ParamParserFactory},
		{Service: staging.ServiceSecret, ParserFactory: staging.SecretParserFactory},
	}
}

// notConfiguredResolver mimics an Azure scope resolver whose resource is not
// named (e.g. no --vault-name), signalling the service should be skipped.
func notConfiguredResolver(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}

// TestRun_SkipUnconfiguredService verifies that a service whose scope is not
// configured is skipped (an unconfigured service can hold no staged state), so a
// provider like Azure with only one of Key Vault / App Configuration connected
// reports no error. Store is nil so each spec resolves via its ScopeResolver.
func TestRun_SkipUnconfiguredService(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	r := &status.Runner{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.ParamParserFactory, ScopeResolver: notConfiguredResolver},
			{Service: staging.ServiceSecret, ParserFactory: staging.SecretParserFactory, ScopeResolver: notConfiguredResolver},
		},
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

// TestRun_ResolverErrorPropagates verifies a non-sentinel resolver error is not
// swallowed by the skip path.
func TestRun_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")

	var buf bytes.Buffer

	r := &status.Runner{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.ParamParserFactory, ScopeResolver: func(_ context.Context) (staging.ResolvedScope, error) {
				return staging.ResolvedScope{}, wantErr
			}},
		},
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.ErrorIs(t, err, wantErr)
}
