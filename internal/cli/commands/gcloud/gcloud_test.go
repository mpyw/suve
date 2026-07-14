package gcloud_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/gcloud"
	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/cli/output"
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

	// RenderJSON emits the structured view over the same fetched entry.
	var jsonBuf bytes.Buffer
	require.NoError(t, presenter.RenderJSON(&jsonBuf, value))

	var showOut struct {
		Name        string            `json:"name"`
		Version     string            `json:"version"`
		State       string            `json:"state"`
		Description string            `json:"description"`
		Created     string            `json:"created"`
		Labels      map[string]string `json:"labels"`
		Value       string            `json:"value"`
	}
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &showOut))
	assert.Equal(t, "my-secret", showOut.Name)
	assert.Equal(t, "3", showOut.Version)
	assert.Equal(t, "enabled", showOut.State)
	assert.Equal(t, "app credentials", showOut.Description)
	assert.Equal(t, "s3cr3t", showOut.Value)
	assert.Equal(t, map[string]string{"env": "prod"}, showOut.Labels)
	assert.NotEmpty(t, showOut.Created)
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

	// RenderValue is a no-op for Google Cloud log (no default value preview).
	var valueBuf bytes.Buffer
	presenter.RenderValue(&valueBuf, 0, 0)
	assert.Empty(t, valueBuf.String())

	// RenderOneline emits a compact per-version line with the state tag.
	var onelineBuf bytes.Buffer
	presenter.RenderOneline(&onelineBuf, 0, 0)
	oneline := onelineBuf.String()
	assert.Contains(t, oneline, "2")
	assert.Contains(t, oneline, "enabled")

	// RenderJSON serializes every version; the destroyed version surfaces its
	// fetch error instead of a value.
	var jsonBuf bytes.Buffer
	require.NoError(t, presenter.RenderJSON(&jsonBuf))

	var items []struct {
		Version string  `json:"version"`
		State   string  `json:"state"`
		Created string  `json:"created"`
		Value   *string `json:"value"`
		Error   string  `json:"error"`
	}
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "2", items[0].Version)
	assert.Equal(t, "enabled", items[0].State)
	require.NotNil(t, items[0].Value)
	assert.Equal(t, "v2", *items[0].Value)
	assert.Empty(t, items[0].Error)
	assert.Equal(t, "1", items[1].Version)
	assert.Equal(t, "destroyed", items[1].State)
	assert.Nil(t, items[1].Value)
	assert.Contains(t, items[1].Error, "cannot access destroyed version")

	// RenderPatch skips versions whose value is inaccessible: i=0 (v2) has a
	// destroyed parent (v1), and i=1 (v1) is itself destroyed. Both branches
	// return without emitting a diff.
	var patchBuf, patchErrBuf bytes.Buffer
	presenter.RenderPatch(&patchBuf, &patchErrBuf, 0, false, false)
	presenter.RenderPatch(&patchBuf, &patchErrBuf, 1, false, false)
	assert.Empty(t, patchBuf.String())
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

// diffVersionSpec builds a Google Cloud diff spec pinned to an integer version.
func diffVersionSpec(v int64) *gcloudversion.Spec {
	return &gcloudversion.Spec{Name: "my-secret", Absolute: gcloudversion.AbsoluteSpec{Version: lo.ToPtr(v)}}
}

// diffStore resolves each spec suffix ("#N") to a ref and returns the mapped
// entry, matching the gcloud diff usecase's resolve-then-get flow.
func diffStore(byRef map[string]*domain.Entry) *providermock.Store {
	return &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			entry, ok := byRef[ref.ID()]
			if !ok {
				return nil, errors.New("version not found")
			}

			return entry, nil
		},
	}
}

func runDiff(
	t *testing.T, presenter genericdiff.Presenter, opts genericdiff.Options,
) (string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	r := &genericdiff.Runner{Presenter: presenter, Options: opts, Stdout: &stdout, Stderr: &stderr}
	err := r.Run(t.Context())

	return stdout.String(), err
}

func TestDiffPresenter(t *testing.T) {
	t.Parallel()

	t.Run("text diff between two versions", func(t *testing.T) {
		t.Parallel()

		store := diffStore(map[string]*domain.Entry{
			"#1": {Name: "my-secret", Value: "old-value", Version: domain.Version{ID: "1"}},
			"#2": {Name: "my-secret", Value: "new-value", Version: domain.Version{ID: "2"}},
		})

		presenter := gcloud.NewDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		out, err := runDiff(t, presenter, genericdiff.Options{})
		require.NoError(t, err)
		assert.Contains(t, out, "-old-value")
		assert.Contains(t, out, "+new-value")
		// Labels carry the Google Cloud "name#version" form.
		assert.Contains(t, out, "my-secret#1")
		assert.Contains(t, out, "my-secret#2")
	})

	t.Run("json output for differing versions", func(t *testing.T) {
		t.Parallel()

		store := diffStore(map[string]*domain.Entry{
			"#1": {Name: "my-secret", Value: "old-value", Version: domain.Version{ID: "1"}},
			"#2": {Name: "my-secret", Value: "new-value", Version: domain.Version{ID: "2"}},
		})

		presenter := gcloud.NewDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		out, err := runDiff(t, presenter, genericdiff.Options{Output: output.FormatJSON})
		require.NoError(t, err)

		var diffOut struct {
			OldName    string `json:"oldName"`
			OldVersion string `json:"oldVersion"`
			OldValue   string `json:"oldValue"`
			NewName    string `json:"newName"`
			NewVersion string `json:"newVersion"`
			NewValue   string `json:"newValue"`
			Identical  bool   `json:"identical"`
			Diff       string `json:"diff"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &diffOut))
		assert.Equal(t, "my-secret", diffOut.OldName)
		assert.Equal(t, "1", diffOut.OldVersion)
		assert.Equal(t, "old-value", diffOut.OldValue)
		assert.Equal(t, "my-secret", diffOut.NewName)
		assert.Equal(t, "2", diffOut.NewVersion)
		assert.Equal(t, "new-value", diffOut.NewValue)
		assert.False(t, diffOut.Identical)
		assert.NotEmpty(t, diffOut.Diff)
	})

	t.Run("fetch error propagates", func(t *testing.T) {
		t.Parallel()

		// Only version 2 is resolvable; fetching spec1 (#1) fails, so Fetch — and
		// thus the whole run — returns the error.
		store := diffStore(map[string]*domain.Entry{
			"#2": {Name: "my-secret", Value: "new-value", Version: domain.Version{ID: "2"}},
		})

		presenter := gcloud.NewDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		_, err := runDiff(t, presenter, genericdiff.Options{})
		require.Error(t, err)
	})

	t.Run("json output for identical versions", func(t *testing.T) {
		t.Parallel()

		store := diffStore(map[string]*domain.Entry{
			"#1": {Name: "my-secret", Value: "same-value", Version: domain.Version{ID: "1"}},
			"#2": {Name: "my-secret", Value: "same-value", Version: domain.Version{ID: "2"}},
		})

		presenter := gcloud.NewDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		out, err := runDiff(t, presenter, genericdiff.Options{Output: output.FormatJSON})
		require.NoError(t, err)

		var diffOut struct {
			Identical bool   `json:"identical"`
			Diff      string `json:"diff"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &diffOut))
		assert.True(t, diffOut.Identical)
		assert.Empty(t, diffOut.Diff)
	})
}
