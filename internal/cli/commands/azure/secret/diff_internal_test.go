// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
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

	t.Run("single spec compares against current", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#abc"})
		require.NoError(t, err)
		assert.Equal(t, new("abc"), spec1.Absolute.ID)
		assert.Nil(t, spec2.Absolute.ID)
	})

	t.Run("two specs", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#abc", "my-secret#def"})
		require.NoError(t, err)
		assert.Equal(t, new("abc"), spec1.Absolute.ID)
		assert.Equal(t, new("def"), spec2.Absolute.ID)
	})

	t.Run("mixed format: full spec plus specifier-only", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#abc", "#def"})
		require.NoError(t, err)
		assert.Equal(t, new("abc"), spec1.Absolute.ID)
		assert.Equal(t, new("def"), spec2.Absolute.ID)
	})

	t.Run("partial spec: name plus specifier-only is swapped", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret", "#abc"})
		require.NoError(t, err)
		assert.Equal(t, new("abc"), spec1.Absolute.ID)
		assert.Nil(t, spec2.Absolute.ID)
	})

	t.Run("three args: name plus two specifiers", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret", "#abc", "#def"})
		require.NoError(t, err)
		assert.Equal(t, new("abc"), spec1.Absolute.ID)
		assert.Equal(t, new("def"), spec2.Absolute.ID)
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

// TestDiffPresenter_RenderJSON drives the Key Vault diff presenter end to end
// through the generic Runner with --output=json, covering newDiffPresenter,
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

	spec1, err := version.AzureKeyVault.Parse("my-secret#abc")
	require.NoError(t, err)
	spec2, err := version.AzureKeyVault.Parse("my-secret#def")
	require.NoError(t, err)

	presenter := newDiffPresenter(store, spec1, spec2)

	var stdout, stderr bytes.Buffer

	r := &generic.DiffRunner{
		Presenter: presenter,
		Options:   generic.DiffOptions{Output: output.FormatJSON},
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
