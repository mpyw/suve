package cli_test

import (
	"bytes"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
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
			Name:          "/app/config",
			Operation:     staging.OperationDelete,
			AWSValue:      "old-value",
			StagedValue:   "",
			AWSIdentifier: "#5",
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
			Name:          "/app/config",
			Operation:     staging.OperationUpdate,
			AWSValue:      `{"b":2,"a":1}`,
			StagedValue:   `{"c":3,"d":4}`,
			AWSIdentifier: "#5",
		}

		r.OutputDiff(cli.DiffOptions{ParseJSON: true}, entry)
		output := stdout.String()
		assert.Contains(t, output, "a")
		assert.Contains(t, output, "b")
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
			Remove: maputil.NewSet[string](),
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
			Remove: maputil.NewSet("deprecated", "old"),
		}

		r.OutputTagEntry(tagEntry)
		output := stdout.String()
		assert.Contains(t, output, "-")
		assert.Contains(t, output, "deprecated")
		assert.Contains(t, output, "old")
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
			Remove: maputil.NewSet("deprecated"),
		}

		r.OutputTagEntry(tagEntry)
		output := stdout.String()
		assert.Contains(t, output, "+")
		assert.Contains(t, output, "-")
	})
}
