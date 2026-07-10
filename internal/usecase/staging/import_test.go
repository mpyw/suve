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

// cannedReader is a fake EnvelopeReader returning pre-seeded per-service states.
// A missing service returns an empty state with a nil error (mirroring an absent
// per-service file in the directory case). An optional err forces a read failure.
type cannedReader struct {
	states map[staging.Service]*staging.State
	err    error
}

func (r *cannedReader) ReadState(_ context.Context, svc staging.Service) (*staging.State, error) {
	if r.err != nil {
		return nil, r.err
	}

	if s, ok := r.states[svc]; ok {
		return s, nil
	}

	return staging.NewEmptyState(), nil
}

// sourceState builds a single-service state carrying one entry, for seeding a
// cannedReader.
func sourceState(svc staging.Service, name, value string) *staging.State {
	s := staging.NewEmptyState()
	s.Entries[svc][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(value),
		StagedAt:  time.Now(),
	}

	return s
}

//nolint:funlen // Many subtests covering merge/overwrite and service/global cases
func TestImportUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("import into empty working - not merged", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "v"),
		}}
		working := testutil.NewMockStore()

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)
		assert.False(t, output.Merged)

		entry, err := working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		assert.Equal(t, "v", lo.FromPtr(entry.Value))
	})

	t.Run("import merge into non-empty working - merged", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/new", "file"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/existing", "work")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Mode: stagingusecase.ImportModeMerge})
		require.NoError(t, err)
		assert.True(t, output.Merged)

		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/existing"})
		require.NoError(t, err)
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new"})
		require.NoError(t, err)
	})

	t.Run("import overwrite replaces working", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "file"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/existing", "work")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Mode: stagingusecase.ImportModeOverwrite})
		require.NoError(t, err)
		assert.False(t, output.Merged)

		// Imported entry present, prior working entry gone.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/existing"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("global overwrite with partial source preserves untouched service", func(t *testing.T) {
		t.Parallel()

		// Source has only param (e.g. a directory where secret.json is absent).
		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "file"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/existing", "old-param")
		stageEntry(t, working, staging.ServiceSecret, "my-secret", "keep-me")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Mode: stagingusecase.ImportModeOverwrite})
		require.NoError(t, err)

		// Param replaced by source.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/existing"})
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Secret was NOT in the source, so it must survive the global overwrite.
		_, err = working.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
		require.NoError(t, err)
	})

	t.Run("import merge conflict - source wins", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "file"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/config", "work")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Mode: stagingusecase.ImportModeMerge})
		require.NoError(t, err)

		entry, err := working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		assert.Equal(t, "file", lo.FromPtr(entry.Value))
	})

	t.Run("nothing to import when source empty", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{}
		working := testutil.NewMockStore()

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{})
		require.ErrorIs(t, err, stagingusecase.ErrNothingToImport)
	})

	t.Run("nothing to import when filtered service empty", func(t *testing.T) {
		t.Parallel()

		// Source only holds secret, but we ask to import param.
		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceSecret: sourceState(staging.ServiceSecret, "my-secret", "s"),
		}}
		working := testutil.NewMockStore()

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Service: staging.ServiceParam})
		require.ErrorIs(t, err, stagingusecase.ErrNothingToImport)
	})

	t.Run("global import merges both services", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam:  sourceState(staging.ServiceParam, "/app/config", "p"),
			staging.ServiceSecret: sourceState(staging.ServiceSecret, "my-secret", "s"),
		}}
		working := testutil.NewMockStore()

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{})
		require.NoError(t, err)
		assert.Equal(t, 2, output.EntryCount)

		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		_, err = working.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
		require.NoError(t, err)
	})

	t.Run("service-specific import preserves other services in working", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "p"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceSecret, "my-secret", "s")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Service: staging.ServiceParam})
		require.NoError(t, err)
		// Working had no param before, so this service-level import is not "merged".
		assert.False(t, output.Merged)

		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		_, err = working.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
		require.NoError(t, err)
	})

	t.Run("service-specific import into non-empty service - merged", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/new", "p"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/existing", "old")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Service: staging.ServiceParam})
		require.NoError(t, err)
		assert.True(t, output.Merged)
	})

	t.Run("service-specific overwrite replaces only that service", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/new", "p"),
		}}
		working := testutil.NewMockStore()
		stageEntry(t, working, staging.ServiceParam, "/app/existing", "old")
		stageEntry(t, working, staging.ServiceSecret, "my-secret", "s")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		output, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{
			Service: staging.ServiceParam,
			Mode:    stagingusecase.ImportModeOverwrite,
		})
		require.NoError(t, err)
		assert.False(t, output.Merged)

		// Old param gone, new param present, secret preserved.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/existing"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new"})
		require.NoError(t, err)
		_, err = working.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
		require.NoError(t, err)
	})
}

func TestImportUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("error on source read", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{err: errors.New("corrupt file")}
		working := testutil.NewMockStore()

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{Service: staging.ServiceParam})

		var importErr *stagingusecase.ImportError
		require.ErrorAs(t, err, &importErr)
		assert.Equal(t, "load", importErr.Op)
	})

	t.Run("error on global source read", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{err: errors.New("corrupt file")}
		working := testutil.NewMockStore()

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		// Global import reads each service; the loop must propagate the read error.
		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{})

		var importErr *stagingusecase.ImportError
		require.ErrorAs(t, err, &importErr)
		assert.Equal(t, "load", importErr.Op)
	})

	t.Run("error on working read", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "v"),
		}}
		working := testutil.NewMockStore()
		working.DrainErr = errors.New("read error")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{})

		var importErr *stagingusecase.ImportError
		require.ErrorAs(t, err, &importErr)
		assert.Equal(t, "read-working", importErr.Op)

		// Working must not have been written.
		_, err = working.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("error on working write", func(t *testing.T) {
		t.Parallel()

		reader := &cannedReader{states: map[staging.Service]*staging.State{
			staging.ServiceParam: sourceState(staging.ServiceParam, "/app/config", "v"),
		}}
		working := testutil.NewMockStore()
		working.WriteStateErr = errors.New("write error")

		usecase := &stagingusecase.ImportUseCase{Source: reader, Working: working}

		_, err := usecase.Execute(t.Context(), stagingusecase.ImportInput{})

		var importErr *stagingusecase.ImportError
		require.ErrorAs(t, err, &importErr)
		assert.Equal(t, "write", importErr.Op)
	})
}

func TestImportError(t *testing.T) {
	t.Parallel()

	t.Run("error message - load", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.ImportError{Op: "load", Err: errors.New("boom")}
		assert.Contains(t, err.Error(), "failed to read export file")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("error message - write", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.ImportError{Op: "write", Err: errors.New("boom")}
		assert.Contains(t, err.Error(), "failed to write the working staging area")
	})

	t.Run("error message - read-working", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.ImportError{Op: "read-working", Err: errors.New("boom")}
		assert.Contains(t, err.Error(), "failed to read the working staging area")
	})

	t.Run("error message - unknown op", func(t *testing.T) {
		t.Parallel()

		inner := errors.New("something went wrong")
		err := &stagingusecase.ImportError{Op: "unknown", Err: inner}
		assert.Equal(t, "something went wrong", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()

		inner := errors.New("inner")
		err := &stagingusecase.ImportError{Op: "load", Err: inner}
		assert.ErrorIs(t, err, inner)
	})
}
