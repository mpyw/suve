package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// recordingWriter is a fake EnvelopeWriter that records the states it is asked
// to write, keyed by service. An optional err forces a write failure.
type recordingWriter struct {
	written map[staging.Service]*staging.State
	err     error
}

func (w *recordingWriter) WriteEnvelope(_ context.Context, svc staging.Service, state *staging.State) error {
	if w.err != nil {
		return w.err
	}

	if w.written == nil {
		w.written = make(map[staging.Service]*staging.State)
	}

	w.written[svc] = state

	return nil
}

func stageEntry(t *testing.T, s *testutil.MockStore, svc staging.Service, name, value string) {
	t.Helper()

	require.NoError(t, s.StageEntry(t.Context(), svc, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(value),
		StagedAt:  time.Now(),
	}))
}

func TestExportUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("nothing to export when working is empty", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		writer := &recordingWriter{}

		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		_, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{})
		require.ErrorIs(t, err, stagingusecase.ErrNothingToExport)
		assert.Empty(t, writer.written)
	})

	t.Run("nothing to export when filtered service is empty", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceSecret, "my-secret", "v")

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		_, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{Service: staging.ServiceParam})
		require.ErrorIs(t, err, stagingusecase.ErrNothingToExport)
		assert.Empty(t, writer.written)
	})

	t.Run("global export writes each non-empty service and clears working", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")
		stageEntry(t, working, staging.ServiceSecret, "my-secret", "s")

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		output, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{})
		require.NoError(t, err)
		assert.Equal(t, 2, output.EntryCount)
		assert.Equal(t, 0, output.TagCount)

		// Both services written, each scoped to its own service only.
		require.Contains(t, writer.written, staging.ServiceParam)
		require.Contains(t, writer.written, staging.ServiceSecret)
		assert.Equal(t, 1, writer.written[staging.ServiceParam].EntryCount())
		assert.Empty(t, writer.written[staging.ServiceParam].Entries[staging.ServiceSecret])

		// The secret envelope carries only the secret entry, never the param one.
		assert.Equal(t, 1, writer.written[staging.ServiceSecret].EntryCount())
		assert.Len(t, writer.written[staging.ServiceSecret].Entries[staging.ServiceSecret], 1)
		assert.Empty(t, writer.written[staging.ServiceSecret].Entries[staging.ServiceParam])

		// Working cleared.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = working.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("global export skips empty service", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		_, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{})
		require.NoError(t, err)

		assert.Contains(t, writer.written, staging.ServiceParam)
		assert.NotContains(t, writer.written, staging.ServiceSecret)
	})

	t.Run("keep preserves the working area", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		_, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{Keep: true})
		require.NoError(t, err)

		// Working still holds the entry.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
	})

	t.Run("service filter clears only that service", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")
		stageEntry(t, working, staging.ServiceSecret, "my-secret", "s")

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		output, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{Service: staging.ServiceParam})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)

		assert.Contains(t, writer.written, staging.ServiceParam)
		assert.NotContains(t, writer.written, staging.ServiceSecret)

		// Param cleared, secret preserved.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = working.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
		require.NoError(t, err)
	})

	t.Run("counts entries and tags", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")

		require.NoError(t, working.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}))

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		output, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{Keep: true})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)
		assert.Equal(t, 1, output.TagCount)
	})
}

func TestExportUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("error on working load", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		working.DrainErr = errors.New("read error")

		usecase := &stagingusecase.ExportUseCase{Working: working, Target: &recordingWriter{}}

		_, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{})

		var exportErr *stagingusecase.ExportError
		require.ErrorAs(t, err, &exportErr)
		assert.Equal(t, stagingusecase.ExportOpLoad, exportErr.Op)
	})

	t.Run("error on target write", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")

		writer := &recordingWriter{err: errors.New("write error")}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		_, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{})

		var exportErr *stagingusecase.ExportError
		require.ErrorAs(t, err, &exportErr)
		assert.Equal(t, stagingusecase.ExportOpWrite, exportErr.Op)

		// Working must not have been cleared: the export failed.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
	})

	t.Run("clear error is non-fatal and output is returned", func(t *testing.T) {
		t.Parallel()

		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "p")

		writer := &recordingWriter{}
		usecase := &stagingusecase.ExportUseCase{Working: working, Target: writer}

		// The export write succeeds; only the working-area clear fails.
		working.WriteStateErr = errors.New("clear error")

		output, err := usecase.Execute(t.Context(), stagingusecase.ExportInput{})
		require.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		var exportErr *stagingusecase.ExportError
		require.ErrorAs(t, err, &exportErr)
		assert.Equal(t, stagingusecase.ExportOpClear, exportErr.Op)
		assert.True(t, exportErr.NonFatal)

		// The state was still exported.
		assert.Contains(t, writer.written, staging.ServiceParam)
	})
}

func TestExportError(t *testing.T) {
	t.Parallel()

	t.Run("error message - load", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.ExportError{Op: stagingusecase.ExportOpLoad, Err: errors.New("boom")}
		assert.Contains(t, err.Error(), "failed to read the working staging area")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("error message - write", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.ExportError{Op: stagingusecase.ExportOpWrite, Err: errors.New("boom")}
		assert.Contains(t, err.Error(), "failed to write export file")
	})

	t.Run("error message - clear", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.ExportError{Op: stagingusecase.ExportOpClear, Err: errors.New("boom")}
		assert.Contains(t, err.Error(), "failed to clear the working staging area")
	})

	t.Run("error message - unknown op", func(t *testing.T) {
		t.Parallel()

		inner := errors.New("something went wrong")
		err := &stagingusecase.ExportError{Op: stagingusecase.ExportOp("unknown"), Err: inner}
		assert.Equal(t, "something went wrong", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()

		inner := errors.New("inner")
		err := &stagingusecase.ExportError{Op: stagingusecase.ExportOpLoad, Err: inner}
		assert.ErrorIs(t, err, inner)
	})
}
