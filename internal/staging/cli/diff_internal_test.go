// These are diff.go's tests: diffEntryDisplayName is private to the diff
// namespace.
//declscope:namespace diff

package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestDiffEntryDisplayName(t *testing.T) {
	t.Parallel()

	// The null/default namespace yields the bare name.
	assert.Equal(t, "app/k", diffEntryDisplayName(stagingusecase.DiffEntry{Name: "app/k"}))

	// A named namespace is appended so a key staged under several namespaces is
	// unambiguous in the diff.
	assert.Equal(t, "app/k [dev]", diffEntryDisplayName(stagingusecase.DiffEntry{Name: "app/k", Namespace: "dev"}))
}

func TestOutputDiff(t *testing.T) {
	t.Parallel()

	t.Run("delete operation diff", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:             "/app/config",
			Operation:        staging.OperationDelete,
			RemoteValue:      "old-value",
			StagedValue:      "",
			RemoteIdentifier: "#5",
		}

		r.outputDiff(diffOptions{}, entry)

		output := stdout.String()
		assert.Contains(t, output, "staged for deletion")
	})

	t.Run("update operation diff with JSON", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:             "/app/config",
			Operation:        staging.OperationUpdate,
			RemoteValue:      `{"b":2,"a":1}`,
			StagedValue:      `{"c":3,"d":4}`,
			RemoteIdentifier: "#5",
		}

		r.outputDiff(diffOptions{parseJSON: true}, entry)

		output := stdout.String()
		assert.Contains(t, output, "a")
		assert.Contains(t, output, "b")
	})

	t.Run("JSON reformat only warns instead of printing an empty diff", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout:        &stdout,
			stderr:        &stderr,
			providerLabel: "Key Vault",
		}

		entry := stagingusecase.DiffEntry{
			Name:        "my-secret",
			Operation:   staging.OperationUpdate,
			RemoteValue: `{"a":1,"b":2}`,
			StagedValue: `{"b":2,"a":1}`,
			Description: new("kept"),
		}

		r.outputDiff(diffOptions{parseJSON: true}, entry)

		assert.Empty(t, stdout.String(), "no diff body or metadata for a formatting-only change")
		assert.Contains(t, stderr.String(), "my-secret: staged value differs from Key Vault only in JSON formatting")
	})
}

func TestOutputDiffCreate(t *testing.T) {
	t.Parallel()

	t.Run("create with JSON formatting", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:        "/app/new-config",
			Operation:   staging.OperationCreate,
			StagedValue: `{"key":"value"}`,
		}

		r.outputDiffCreate(diffOptions{parseJSON: true}, entry)

		output := stdout.String()
		assert.Contains(t, output, "staged for creation")
		assert.Contains(t, output, "key")
	})

	t.Run("create with non-JSON value", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:        "/app/new-config",
			Operation:   staging.OperationCreate,
			StagedValue: "plain-text-value",
		}

		r.outputDiffCreate(diffOptions{parseJSON: true}, entry)

		output := stdout.String()
		assert.Contains(t, output, "plain-text-value")
	})
}

func TestOutputMetadata(t *testing.T) {
	t.Parallel()

	t.Run("with description", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Description: new("Test description"),
		}

		r.outputMetadata(entry)

		output := stdout.String()
		assert.Contains(t, output, "Description:")
		assert.Contains(t, output, "Test description")
	})

	t.Run("without description", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Description: nil,
		}

		r.outputMetadata(entry)
		assert.Empty(t, stdout.String())
	})

	t.Run("with empty description", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Description: new(""),
		}

		r.outputMetadata(entry)
		assert.Empty(t, stdout.String())
	})
}

func TestOutputTagEntry(t *testing.T) {
	t.Parallel()

	t.Run("add tags only", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		tagEntry := stagingusecase.DiffTagEntry{
			Name:   "/app/config",
			Add:    map[string]string{"env": "prod"},
			Remove: map[string]string{},
		}

		r.outputTagEntry(tagEntry)

		output := stdout.String()
		assert.Contains(t, output, "staged tag changes")
		assert.Contains(t, output, "+")
		assert.Contains(t, output, "env=prod")
	})

	t.Run("remove tags only", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		tagEntry := stagingusecase.DiffTagEntry{
			Name:   "/app/config",
			Add:    map[string]string{},
			Remove: map[string]string{"deprecated": "true", "old": "legacy"},
		}

		r.outputTagEntry(tagEntry)

		output := stdout.String()
		assert.Contains(t, output, "-")
		assert.Contains(t, output, "deprecated=true")
		assert.Contains(t, output, "old=legacy")
	})

	t.Run("namespaced tags carry the namespace badge", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		r.outputTagEntry(stagingusecase.DiffTagEntry{
			Name:      "app/key",
			Namespace: "dev",
			Add:       map[string]string{"env": "dev"},
		})

		assert.Contains(t, stdout.String(), "app/key [dev] (staged tag changes)")
	})

	t.Run("both add and remove tags", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &diffRunner{
			stdout: &stdout,
			stderr: &stderr,
		}

		tagEntry := stagingusecase.DiffTagEntry{
			Name:   "/app/config",
			Add:    map[string]string{"env": "prod"},
			Remove: map[string]string{"deprecated": "old-value"},
		}

		r.outputTagEntry(tagEntry)

		output := stdout.String()
		assert.Contains(t, output, "+")
		assert.Contains(t, output, "-")
	})
}

// TestDiffRunner_TagsUnderSeveralNamespaces verifies the same key tagged under
// two App Configuration namespaces renders two tag blocks, each with its own
// namespace badge, instead of collapsing onto one.
func TestDiffRunner_TagsUnderSeveralNamespaces(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "k"}, staging.TagEntry{
		Add: map[string]string{"a": "1"}, StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "k", Namespace: "dev"}, staging.TagEntry{
		Add: map[string]string{"b": "2"}, StagedAt: time.Now(),
	}))

	var stdout, stderr bytes.Buffer

	r := &diffRunner{
		useCase: &stagingusecase.DiffUseCase{Strategy: &cliFullMockStrategy{service: staging.ServiceParam}, Store: store},
		stdout:  &stdout,
		stderr:  &stderr,
	}

	require.NoError(t, r.run(t.Context(), diffOptions{}))

	out := stdout.String()
	assert.Contains(t, out, "k (staged tag changes)")
	assert.Contains(t, out, "a=1")
	assert.Contains(t, out, "k [dev] (staged tag changes)")
	assert.Contains(t, out, "b=2")
}
