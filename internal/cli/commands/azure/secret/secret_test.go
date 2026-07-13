package secret_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/azure/secret"
	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azurekvversion"
)

// TestCommandValidation exercises argument/spec validation that fails before any
// provider store is resolved (so no Azure credentials are needed).
func TestCommandValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "create missing args",
			args:    []string{"suve", "azure", "secret", "create"},
			wantErr: "usage:",
		},
		{
			name:    "update missing name",
			args:    []string{"suve", "azure", "secret", "update"},
			wantErr: "usage:",
		},
		{
			// The value is now optional (stdin/editor fallback), but a positional
			// value cannot be combined with --value-stdin.
			name:    "create value with --value-stdin conflicts",
			args:    []string{"suve", "azure", "secret", "create", "my-secret", "value", "--value-stdin"},
			wantErr: "cannot combine a positional value with --value-stdin",
		},
		{
			name:    "delete missing name",
			args:    []string{"suve", "azure", "secret", "delete"},
			wantErr: "usage:",
		},
		{
			name:    "show missing name",
			args:    []string{"suve", "azure", "secret", "show"},
			wantErr: "usage:",
		},
		{
			name:    "show rejects label spec",
			args:    []string{"suve", "azure", "secret", "show", "my-secret:latest"},
			wantErr: "staging labels are not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCreateRunner(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "my-secret", name)
			assert.Equal(t, "value", value)
			assert.Equal(t, domain.ValueTypeSecret, vt)

			return domain.Version{ID: "v1"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &secret.CreateRunner{
		UseCase: &azure.CreateUseCase{Writer: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), secret.CreateOptions{Name: "my-secret", Value: "value"}))
	assert.Contains(t, buf.String(), "Created secret my-secret")
	assert.Contains(t, buf.String(), "version: v1")
}

func TestUpdateRunner_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	var buf, errBuf bytes.Buffer

	r := &secret.UpdateRunner{
		UseCase: &azure.UpdateUseCase{Store: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	err := r.Run(t.Context(), secret.UpdateOptions{Name: "missing", Value: "new"})
	require.ErrorIs(t, err, azure.ErrEntryNotFound)
}

func TestShowPresenter(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "s3cr3t",
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: "abc123", State: "enabled", Created: &created},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := azurekvversion.Parse("my-secret")
	require.NoError(t, err)

	presenter := secret.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	presenter.RenderText(&buf, value)

	out := buf.String()
	assert.Contains(t, out, "my-secret")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "s3cr3t")
	assert.Contains(t, out, "env")
}

func TestLogPresenter(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "new", State: "enabled", Created: &created},
				{ID: "old", State: "disabled", Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			if ref.ID() == "old" {
				return nil, errors.New("disabled version has no value")
			}

			return &domain.Entry{Value: "v-" + ref.ID()}, nil
		},
	}

	presenter := secret.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))
	assert.Equal(t, 2, presenter.Len())

	var buf bytes.Buffer

	presenter.RenderHeader(&buf, 0)
	out := buf.String()
	assert.Contains(t, out, "Version new")
	assert.Contains(t, out, "enabled")
}

func TestLogPresenter_Patch(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "2", State: "enabled", Created: &created},
				{ID: "1", State: "enabled", Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "v" + ref.ID()}, nil
		},
	}

	presenter := secret.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	// i=0 is the newest version (v2): patch against its parent v1.
	presenter.RenderPatch(&buf, &errBuf, 0, false, false)
	// i=1 is the oldest/initial version (v1): all-added creation diff.
	presenter.RenderPatch(&buf, &errBuf, 1, false, false)

	out := buf.String()
	assert.Contains(t, out, "-v1")
	assert.Contains(t, out, "+v2")
	assert.Contains(t, out, "my-secret#1")
	assert.Contains(t, out, "my-secret#2")
	// The initial version renders its all-added creation diff (+v1).
	assert.Contains(t, out, "+v1")
}

// TestShowPresenter_RenderJSON covers the Key Vault show presenter's JSON path:
// name/version/state/created plus the tags map and value.
func TestShowPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "s3cr3t",
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: "abc123", State: "enabled", Created: &created},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := azurekvversion.Parse("my-secret")
	require.NoError(t, err)

	presenter := secret.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	require.NoError(t, presenter.RenderJSON(&buf, value))

	var out struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		State   string            `json:"state"`
		Created string            `json:"created"`
		Tags    map[string]string `json:"tags"`
		Value   string            `json:"value"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "my-secret", out.Name)
	assert.Equal(t, "abc123", out.Version)
	assert.Equal(t, "enabled", out.State)
	assert.Equal(t, timeutil.FormatRFC3339(created), out.Created)
	assert.Equal(t, "s3cr3t", out.Value)
	assert.Equal(t, map[string]string{"env": "prod"}, out.Tags)
}

// TestDiffPresenter_RenderJSON drives the Key Vault diff presenter end to end
// through the generic Runner with --output=json, covering NewDiffPresenter,
// Fetch, OldValue/NewValue, Labels, and RenderJSON.
func TestDiffPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	values := map[string]string{"abc": "old-val", "def": "new-val"}

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   values[ref.ID()],
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}

	spec1, err := azurekvversion.Parse("my-secret#abc")
	require.NoError(t, err)
	spec2, err := azurekvversion.Parse("my-secret#def")
	require.NoError(t, err)

	presenter := secret.NewDiffPresenter(store, spec1, spec2)

	var stdout, stderr bytes.Buffer

	r := &genericdiff.Runner{
		Presenter: presenter,
		Options:   genericdiff.Options{Output: output.FormatJSON},
		Stdout:    &stdout,
		Stderr:    &stderr,
	}
	require.NoError(t, r.Run(t.Context()))

	var out struct {
		OldName    string `json:"oldName"`
		OldVersion string `json:"oldVersion"`
		OldValue   string `json:"oldValue"`
		NewName    string `json:"newName"`
		NewVersion string `json:"newVersion"`
		NewValue   string `json:"newValue"`
		Identical  bool   `json:"identical"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, "my-secret", out.OldName)
	assert.Equal(t, "abc", out.OldVersion)
	assert.Equal(t, "old-val", out.OldValue)
	assert.Equal(t, "my-secret", out.NewName)
	assert.Equal(t, "def", out.NewVersion)
	assert.Equal(t, "new-val", out.NewValue)
	assert.False(t, out.Identical)
}

// logStore builds a Key Vault log store from a newest-first version history.
// A version listed in errVersions has its value fetch fail (mirroring a disabled
// version, whose value is inaccessible).
func logStore(history []domain.Version, errVersions ...string) *providermock.Store {
	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return history, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			if slices.Contains(errVersions, ref.ID()) {
				return nil, errors.New("disabled version has no accessible value")
			}

			return &domain.Entry{Value: "v-" + ref.ID()}, nil
		},
	}
}

// TestLogPresenter_RenderJSON covers the JSON path, including both the value
// branch (enabled versions) and the error branch (disabled versions) plus tags.
func TestLogPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := logStore([]domain.Version{
		{ID: "new", State: "enabled", Created: &created, Tags: []domain.Tag{{Key: "env", Value: "prod"}}},
		{ID: "old", State: "disabled", Created: &created},
	}, "old")

	presenter := secret.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf bytes.Buffer
	require.NoError(t, presenter.RenderJSON(&buf))

	var items []struct {
		Version string            `json:"version"`
		State   string            `json:"state"`
		Created string            `json:"created"`
		Value   *string           `json:"value"`
		Tags    map[string]string `json:"tags"`
		Error   string            `json:"error"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 2)

	assert.Equal(t, "new", items[0].Version)
	assert.Equal(t, "enabled", items[0].State)
	require.NotNil(t, items[0].Value)
	assert.Equal(t, "v-new", *items[0].Value)
	assert.Equal(t, map[string]string{"env": "prod"}, items[0].Tags)
	assert.Empty(t, items[0].Error)

	assert.Equal(t, "old", items[1].Version)
	assert.Nil(t, items[1].Value)
	assert.Contains(t, items[1].Error, "disabled version")
}

// TestLogPresenter_RenderOneline covers the compact one-line format, including
// the state annotation and the formatted date.
func TestLogPresenter_RenderOneline(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := logStore([]domain.Version{
		{ID: "new", State: "enabled", Created: &created},
	})

	presenter := secret.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf bytes.Buffer
	presenter.RenderOneline(&buf, 0, 0)
	// RenderValue is a documented no-op; call it to confirm it stays silent.
	presenter.RenderValue(&buf, 0, 0)

	out := buf.String()
	assert.Contains(t, out, "new")
	assert.Contains(t, out, "enabled")
	assert.Contains(t, out, "2024-05-06")
}

// TestLogPresenter_PatchSkips covers the three RenderPatch skip branches:
//   - the newest entry is disabled, so its value is missing (log.go:154);
//   - the parent entry is disabled, so the old value is missing (log.go:180);
//   - the oldest shown entry is not the initial version because the window was
//     cut by --number, so no all-added creation diff is emitted (log.go:165).
func TestLogPresenter_PatchSkips(t *testing.T) {
	t.Parallel()

	t.Run("disabled newest and disabled parent are skipped", func(t *testing.T) {
		t.Parallel()

		// History: new (enabled) -> old (disabled). Both branches are exercised:
		// i=0 has a disabled parent (old); i=1 is itself disabled.
		store := logStore([]domain.Version{
			{ID: "new", State: "enabled"},
			{ID: "old", State: "disabled"},
		}, "old")

		presenter := secret.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
		require.NoError(t, presenter.Fetch(t.Context()))

		var buf, errBuf bytes.Buffer

		presenter.RenderPatch(&buf, &errBuf, 0, false, false) // parent "old" is disabled -> skip
		presenter.RenderPatch(&buf, &errBuf, 1, false, false) // "old" itself is disabled -> skip

		assert.Empty(t, buf.String())
	})

	t.Run("windowed oldest is not the initial version", func(t *testing.T) {
		t.Parallel()

		// Full history has two versions but --number=1 shows only the newest, so
		// the oldest shown version is not the genuine initial: no creation diff.
		store := logStore([]domain.Version{
			{ID: "new", State: "enabled"},
			{ID: "old", State: "enabled"},
		})

		presenter := secret.NewLogPresenter(store, genericlog.Request{Name: "my-secret", MaxResults: 1})
		require.NoError(t, presenter.Fetch(t.Context()))
		require.Equal(t, 1, presenter.Len())

		var buf, errBuf bytes.Buffer
		presenter.RenderPatch(&buf, &errBuf, 0, false, false)

		assert.Empty(t, buf.String())
	})
}
