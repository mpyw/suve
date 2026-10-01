// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/version"
)

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	t.Run("single spec compares against latest", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#3"})
		require.NoError(t, err)
		assert.Equal(t, new(int64(3)), spec1.Absolute.Version)
		assert.Nil(t, spec2.Absolute.Version)
	})

	t.Run("two specs", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#1", "my-secret#2"})
		require.NoError(t, err)
		assert.Equal(t, new(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, new(int64(2)), spec2.Absolute.Version)
	})

	t.Run("mixed format: full spec plus specifier-only", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#1", "#2"})
		require.NoError(t, err)
		assert.Equal(t, new(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, new(int64(2)), spec2.Absolute.Version)
	})

	t.Run("partial spec: name plus specifier-only is swapped", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret", "#3"})
		require.NoError(t, err)
		assert.Equal(t, new(int64(3)), spec1.Absolute.Version)
		assert.Nil(t, spec2.Absolute.Version)
	})

	t.Run("three args: name plus two specifiers", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret", "#1", "#2"})
		require.NoError(t, err)
		assert.Equal(t, new(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, new(int64(2)), spec2.Absolute.Version)
	})

	t.Run("no args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{})
		require.Error(t, err)
	})

	t.Run("too many args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{"a", "b", "c", "d"})
		require.Error(t, err)
	})

	t.Run("label rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{"my-secret:latest"})
		require.Error(t, err)
	})
}

func TestDiffPresenter(t *testing.T) {
	t.Parallel()

	t.Run("text diff between two versions", func(t *testing.T) {
		t.Parallel()

		store := diffStore(map[string]*domain.Entry{
			"#1": {Name: "my-secret", Value: "old-value", Version: domain.Version{ID: "1"}},
			"#2": {Name: "my-secret", Value: "new-value", Version: domain.Version{ID: "2"}},
		})

		presenter := newDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		out, err := runDiff(t, presenter, generic.DiffOptions{})
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

		presenter := newDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		out, err := runDiff(t, presenter, generic.DiffOptions{Output: output.FormatJSON})
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

		presenter := newDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		_, err := runDiff(t, presenter, generic.DiffOptions{})
		require.Error(t, err)
	})

	t.Run("json output for identical versions", func(t *testing.T) {
		t.Parallel()

		store := diffStore(map[string]*domain.Entry{
			"#1": {Name: "my-secret", Value: "same-value", Version: domain.Version{ID: "1"}},
			"#2": {Name: "my-secret", Value: "same-value", Version: domain.Version{ID: "2"}},
		})

		presenter := newDiffPresenter(store, diffVersionSpec(1), diffVersionSpec(2))
		out, err := runDiff(t, presenter, generic.DiffOptions{Output: output.FormatJSON})
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

// diffVersionSpec builds a Google Cloud diff spec pinned to an integer version.
func diffVersionSpec(v int64) *version.NumericSpec {
	return &version.NumericSpec{Name: "my-secret", Absolute: version.NumericAbsolute{Version: new(v)}}
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
	t *testing.T, presenter generic.DiffPresenter, opts generic.DiffOptions,
) (string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	r := &generic.DiffRunner{Presenter: presenter, Options: opts, Stdout: &stdout, Stderr: &stderr}
	err := r.Run(t.Context())

	return stdout.String(), err
}
