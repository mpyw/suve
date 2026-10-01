// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package param

import (
	"bytes"
	"context"
	"encoding/json"
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

	t.Run("single bare key compares against itself", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"key-a"})
		require.NoError(t, err)
		assert.Equal(t, "key-a", spec1.Name)
		assert.Equal(t, "key-a", spec2.Name)
	})

	t.Run("two bare keys compared", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"key-a", "key-b"})
		require.NoError(t, err)
		assert.Equal(t, "key-a", spec1.Name)
		assert.Equal(t, "key-b", spec2.Name)
	})

	t.Run("single key containing hash compares against itself", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-key#1"})
		require.NoError(t, err)
		assert.Equal(t, "my-key#1", spec1.Name)
		assert.Equal(t, "my-key#1", spec2.Name)
	})

	t.Run("two keys where the second contains a hash", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-key", "#1"})
		require.NoError(t, err)
		assert.Equal(t, "my-key", spec1.Name)
		assert.Equal(t, "#1", spec2.Name)
	})

	t.Run("three args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{"my-key", "#1", "#2"})
		require.Error(t, err)
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
}

// TestDiffPresenter_RenderJSON drives the App Configuration diff presenter end to
// end through the generic Runner with --output=json, covering newDiffPresenter,
// Fetch, OldValue/NewValue, Labels, and RenderJSON. App Configuration is
// unversioned, so diff compares two distinct keys with an empty suffix.
func TestDiffPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	values := map[string]string{"key-a": "alpha", "key-b": "beta"}

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec) // App Configuration is unversioned.

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: values[name]}, nil
		},
	}

	spec1, err := version.AzureAppConfiguration.Parse("key-a")
	require.NoError(t, err)
	spec2, err := version.AzureAppConfiguration.Parse("key-b")
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
		OldName   string `json:"oldName"`
		OldValue  string `json:"oldValue"`
		NewName   string `json:"newName"`
		NewValue  string `json:"newValue"`
		Identical bool   `json:"identical"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, "key-a", out.OldName)
	assert.Equal(t, "alpha", out.OldValue)
	assert.Equal(t, "key-b", out.NewName)
	assert.Equal(t, "beta", out.NewValue)
	assert.False(t, out.Identical)
}
