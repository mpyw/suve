package gcloud_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/gcloud"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	gcloudusecase "github.com/mpyw/suve/internal/usecase/gcloud"
	"github.com/mpyw/suve/internal/version/gcloudversion"
)

// TestCommandValidation exercises argument/spec validation that fails before any
// provider store is resolved (so no Google Cloud credentials are needed).
func TestCommandValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "create missing args",
			args:    []string{"suve", "gcloud", "secret", "create"},
			wantErr: "usage:",
		},
		{
			name:    "update missing name",
			args:    []string{"suve", "gcloud", "secret", "update"},
			wantErr: "usage:",
		},
		{
			// The value is now optional (stdin/editor fallback), but a positional
			// value cannot be combined with --value-stdin.
			name:    "create value with --value-stdin conflicts",
			args:    []string{"suve", "gcloud", "secret", "create", "my-secret", "value", "--value-stdin"},
			wantErr: "cannot combine a positional value with --value-stdin",
		},
		{
			name:    "delete missing name",
			args:    []string{"suve", "gcloud", "secret", "delete"},
			wantErr: "usage:",
		},
		{
			name:    "show missing name",
			args:    []string{"suve", "gcloud", "secret", "show"},
			wantErr: "usage:",
		},
		{
			name:    "show rejects label spec",
			args:    []string{"suve", "gcloud", "secret", "show", "my-secret:latest"},
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

	var gotDescription string

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, value string, vt domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "my-secret", name)
			assert.Equal(t, "value", value)
			assert.Equal(t, domain.ValueTypeSecret, vt)

			gotDescription = description

			return domain.Version{ID: "1"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &gcloud.CreateRunner{
		UseCase: &gcloudusecase.CreateUseCase{Writer: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), gcloud.CreateOptions{Name: "my-secret", Value: "value", Description: "app credentials"}))
	assert.Contains(t, buf.String(), "Created secret my-secret")
	assert.Contains(t, buf.String(), "version: 1")
	assert.Equal(t, "app credentials", gotDescription, "the --description value reaches the writer")
}

func TestUpdateRunner(t *testing.T) {
	t.Parallel()

	var gotDescription string

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: "my-secret", Value: "old"}, nil
		},
		PutFunc: func(
			_ context.Context, _, value string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "new", value)

			gotDescription = description

			return domain.Version{ID: "2"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &gcloud.UpdateRunner{
		UseCase: &gcloudusecase.UpdateUseCase{Store: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), gcloud.UpdateOptions{Name: "my-secret", Value: "new", Description: "rotated key"}))
	assert.Contains(t, buf.String(), "Updated secret my-secret")
	assert.Contains(t, buf.String(), "version: 2")
	assert.Equal(t, "rotated key", gotDescription, "the --description value reaches the writer")
}

func TestUpdateRunner_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	var buf, errBuf bytes.Buffer

	r := &gcloud.UpdateRunner{
		UseCase: &gcloudusecase.UpdateUseCase{Store: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	err := r.Run(t.Context(), gcloud.UpdateOptions{Name: "missing", Value: "new"})
	require.ErrorIs(t, err, gcloudusecase.ErrSecretNotFound)
}

func TestDeleteRunner(t *testing.T) {
	t.Parallel()

	var deleted string

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
			deleted = name

			return nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &gcloud.DeleteRunner{
		UseCase: &gcloudusecase.DeleteUseCase{Store: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), gcloud.DeleteOptions{Name: "my-secret"}))
	assert.Equal(t, "my-secret", deleted)
	assert.Contains(t, buf.String(), "Permanently deleted secret my-secret")
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
				Name:        name,
				Value:       "s3cr3t",
				Type:        domain.ValueTypeSecret,
				Version:     domain.Version{ID: "3", State: "enabled", Created: &created},
				Description: "app credentials",
				Tags:        []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := gcloudversion.Parse("my-secret")
	require.NoError(t, err)

	presenter := gcloud.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	presenter.RenderText(&buf, value)

	out := buf.String()
	assert.Contains(t, out, "my-secret")
	assert.Contains(t, out, "Version")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "s3cr3t")
	assert.Contains(t, out, "env")
	// The "description" annotation surfaces as a Description field in the read view.
	assert.Contains(t, out, "Description")
	assert.Contains(t, out, "app credentials")
	// No ARN or Stages fields for Google Cloud.
	assert.NotContains(t, out, "ARN")
	assert.NotContains(t, out, "Stages")
}

func TestLogPresenter(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "2", State: "enabled", Created: &created},
				{ID: "1", State: "destroyed", Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			// getValue resolves "#<version>".
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			if ref.ID() == "1" {
				// Destroyed version value is inaccessible.
				return nil, errors.New("cannot access destroyed version")
			}

			return &domain.Entry{Value: "v" + ref.ID()}, nil
		},
	}

	presenter := gcloud.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))
	assert.Equal(t, 2, presenter.Len())

	var buf bytes.Buffer

	presenter.RenderHeader(&buf, 0)
	out := buf.String()
	assert.Contains(t, out, "Version 2")
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

	presenter := gcloud.NewLogPresenter(store, genericlog.Request{Name: "my-secret"})
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
