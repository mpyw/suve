package cli_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestOutputDiff(t *testing.T) {
	t.Parallel()

	t.Run("delete operation diff", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:             "/app/config",
			Operation:        staging.OperationDelete,
			RemoteValue:      "old-value",
			StagedValue:      "",
			RemoteIdentifier: "#5",
		}

		r.OutputDiff(cli.DiffOptions{}, entry)

		output := stdout.String()
		assert.Contains(t, output, "staged for deletion")
	})

	t.Run("update operation diff with JSON", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:             "/app/config",
			Operation:        staging.OperationUpdate,
			RemoteValue:      `{"b":2,"a":1}`,
			StagedValue:      `{"c":3,"d":4}`,
			RemoteIdentifier: "#5",
		}

		r.OutputDiff(cli.DiffOptions{ParseJSON: true}, entry)

		output := stdout.String()
		assert.Contains(t, output, "a")
		assert.Contains(t, output, "b")
	})

	t.Run("JSON reformat only warns instead of printing an empty diff", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout:      &stdout,
			Stderr:      &stderr,
			RemoteLabel: "Key Vault",
		}

		entry := stagingusecase.DiffEntry{
			Name:        "my-secret",
			Operation:   staging.OperationUpdate,
			RemoteValue: `{"a":1,"b":2}`,
			StagedValue: `{"b":2,"a":1}`,
			Description: lo.ToPtr("kept"),
		}

		r.OutputDiff(cli.DiffOptions{ParseJSON: true}, entry)

		assert.Empty(t, stdout.String(), "no diff body or metadata for a formatting-only change")
		assert.Contains(t, stderr.String(), "my-secret: staged value differs from Key Vault only in JSON formatting")
	})
}

func TestOutputDiffCreate(t *testing.T) {
	t.Parallel()

	t.Run("create with JSON formatting", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:        "/app/new-config",
			Operation:   staging.OperationCreate,
			StagedValue: `{"key":"value"}`,
		}

		r.OutputDiffCreate(cli.DiffOptions{ParseJSON: true}, entry)

		output := stdout.String()
		assert.Contains(t, output, "staged for creation")
		assert.Contains(t, output, "key")
	})

	t.Run("create with non-JSON value", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Name:        "/app/new-config",
			Operation:   staging.OperationCreate,
			StagedValue: "plain-text-value",
		}

		r.OutputDiffCreate(cli.DiffOptions{ParseJSON: true}, entry)

		output := stdout.String()
		assert.Contains(t, output, "plain-text-value")
	})
}

func TestOutputMetadata(t *testing.T) {
	t.Parallel()

	t.Run("with description", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Description: lo.ToPtr("Test description"),
		}

		r.OutputMetadata(entry)

		output := stdout.String()
		assert.Contains(t, output, "Description:")
		assert.Contains(t, output, "Test description")
	})

	t.Run("without description", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Description: nil,
		}

		r.OutputMetadata(entry)
		assert.Empty(t, stdout.String())
	})

	t.Run("with empty description", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		entry := stagingusecase.DiffEntry{
			Description: lo.ToPtr(""),
		}

		r.OutputMetadata(entry)
		assert.Empty(t, stdout.String())
	})
}

func TestOutputTagEntry(t *testing.T) {
	t.Parallel()

	t.Run("add tags only", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		tagEntry := stagingusecase.DiffTagEntry{
			Name:   "/app/config",
			Add:    map[string]string{"env": "prod"},
			Remove: map[string]string{},
		}

		r.OutputTagEntry(tagEntry)

		output := stdout.String()
		assert.Contains(t, output, "staged tag changes")
		assert.Contains(t, output, "+")
		assert.Contains(t, output, "env=prod")
	})

	t.Run("remove tags only", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		tagEntry := stagingusecase.DiffTagEntry{
			Name:   "/app/config",
			Add:    map[string]string{},
			Remove: map[string]string{"deprecated": "true", "old": "legacy"},
		}

		r.OutputTagEntry(tagEntry)

		output := stdout.String()
		assert.Contains(t, output, "-")
		assert.Contains(t, output, "deprecated=true")
		assert.Contains(t, output, "old=legacy")
	})

	t.Run("namespaced tags carry the namespace badge", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		r.OutputTagEntry(stagingusecase.DiffTagEntry{
			Name:      "app/key",
			Namespace: "dev",
			Add:       map[string]string{"env": "dev"},
		})

		assert.Contains(t, stdout.String(), "app/key [dev] (staged tag changes)")
	})

	t.Run("both add and remove tags", func(t *testing.T) {
		t.Parallel()

		var stdout, stderr bytes.Buffer

		r := &cli.DiffRunner{
			Stdout: &stdout,
			Stderr: &stderr,
		}

		tagEntry := stagingusecase.DiffTagEntry{
			Name:   "/app/config",
			Add:    map[string]string{"env": "prod"},
			Remove: map[string]string{"deprecated": "old-value"},
		}

		r.OutputTagEntry(tagEntry)

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

	r := &cli.DiffRunner{
		UseCase: &stagingusecase.DiffUseCase{Strategy: &fullMockStrategy{service: staging.ServiceParam}, Store: store},
		Stdout:  &stdout,
		Stderr:  &stderr,
	}

	require.NoError(t, r.Run(t.Context(), cli.DiffOptions{}))

	out := stdout.String()
	assert.Contains(t, out, "k (staged tag changes)")
	assert.Contains(t, out, "a=1")
	assert.Contains(t, out, "k [dev] (staged tag changes)")
	assert.Contains(t, out, "b=2")
}
