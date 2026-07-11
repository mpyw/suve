package secret_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/azure/secret"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
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
