// White-box tests of add.go.
//declscope:namespace add

package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestAddRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("create new item", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var buf bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout:     &buf,
			stderr:     &bytes.Buffer{},
			openEditor: func(_ context.Context, _ string) (string, error) { return "new-value", nil },
		}

		err := r.run(t.Context(), addOptions{name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Staged for creation")
		assert.Contains(t, buf.String(), "/app/config")

		// Verify staged with OperationCreate
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
	})

	t.Run("edit already staged create", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		// Pre-stage as create
		_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     new("original-value"),
		})

		var buf bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout: &buf,
			stderr: &bytes.Buffer{},
			openEditor: func(_ context.Context, current string) (string, error) {
				assert.Equal(t, "original-value", current)

				return "updated-value", nil
			},
		}

		err := r.run(t.Context(), addOptions{name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Staged for creation")

		// Verify updated
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "updated-value", lo.FromPtr(entry.Value))
	})

	t.Run("empty value not staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var buf bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout:     &buf,
			stderr:     &bytes.Buffer{},
			openEditor: func(_ context.Context, _ string) (string, error) { return "", nil },
		}

		err := r.run(t.Context(), addOptions{name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Empty value")

		// Verify not staged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
		assert.Equal(t, staging.ErrNotStaged, err)
	})

	t.Run("no changes made", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		// Pre-stage as create
		_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     new("same-value"),
		})

		var buf bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout: &buf,
			stderr: &bytes.Buffer{},
			openEditor: func(_ context.Context, _ string) (string, error) {
				return "same-value", nil
			},
		}

		err := r.run(t.Context(), addOptions{name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No changes made")
	})

	t.Run("Secrets Manager service", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var buf bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceSecret},
				Store:    store,
			},
			stdout:     &buf,
			stderr:     &bytes.Buffer{},
			openEditor: func(_ context.Context, _ string) (string, error) { return "secret-value", nil },
		}

		err := r.run(t.Context(), addOptions{name: "my-secret"})
		require.NoError(t, err)

		// Verify staged with correct service
		entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "secret-value", lo.FromPtr(entry.Value))
	})
}

// mockStrategy implements staging.EditStrategy for testing.
type addMockStrategy struct {
	service      staging.Service
	parseNameErr error
}

func (m *addMockStrategy) Service() staging.Service { return m.service }
func (m *addMockStrategy) ServiceName() string      { return string(m.service) }
func (m *addMockStrategy) ItemName() string         { return "item" }
func (m *addMockStrategy) HasDeleteOptions() bool   { return false }
func (m *addMockStrategy) ParseName(input string) (string, error) {
	if m.parseNameErr != nil {
		return "", m.parseNameErr
	}

	return input, nil
}
func (m *addMockStrategy) ParseSpec(input string) (string, bool, error) {
	return input, false, nil
}

// FetchCurrentValue returns not-found for add scenarios (new resource).
func (m *addMockStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	return nil, &staging.ResourceNotFoundError{Err: errors.New("resource not found")}
}

func TestAddRunner_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("parse name error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam, parseNameErr: errors.New("invalid name")},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), addOptions{name: "invalid"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid name")
	})

	t.Run("editor error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
			openEditor: func(_ context.Context, _ string) (string, error) {
				return "", errors.New("editor crashed")
			},
		}

		err := r.run(t.Context(), addOptions{name: "/app/config"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit")
	})
}

func TestAddRunner_WithOptions(t *testing.T) {
	t.Parallel()

	t.Run("with provided value (skip editor)", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
			// No OpenEditor set - with Value provided, editor should not be called
		}

		err := r.run(t.Context(), addOptions{
			name:     "/app/new-config",
			value:    "direct-value",
			hasValue: true,
		})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged for creation")

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-config", Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "direct-value", lo.FromPtr(entry.Value))
	})

	t.Run("with description", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &addRunner{
			useCase: &stagingusecase.AddUseCase{
				Strategy: &addMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), addOptions{
			name:        "/app/new-config",
			value:       "test-value",
			hasValue:    true,
			description: "Test description",
		})
		require.NoError(t, err)

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-config", Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, "Test description", lo.FromPtr(entry.Description))
	})
}

// TestAddEditRunner_ValueSources covers the value sources of stage add/edit
// (#985): without a terminal and without an injected editor, a missing value
// fails instead of launching $EDITOR; an explicit "" is a value; --value-stdin
// reads the value from stdin.
func TestAddEditRunner_ValueSources(t *testing.T) {
	t.Parallel()

	newAdd := func(store *testutil.MockStore, stdin *bytes.Buffer) *addRunner {
		return &addRunner{
			useCase: &stagingusecase.AddUseCase{Strategy: &addMockStrategy{service: staging.ServiceParam}, Store: store},
			stdout:  &bytes.Buffer{},
			stderr:  &bytes.Buffer{},
			stdin:   stdin,
		}
	}
	newEdit := func(store *testutil.MockStore, stdin *bytes.Buffer) *editRunner {
		return &editRunner{
			useCase: &stagingusecase.EditUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "remote-value"},
				Store:    store,
			},
			stdout: &bytes.Buffer{},
			stderr: &bytes.Buffer{},
			stdin:  stdin,
		}
	}
	stagedValue := func(t *testing.T, store *testutil.MockStore) string {
		t.Helper()

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)

		return lo.FromPtr(entry.Value)
	}

	t.Run("add without value on a non-TTY stdin fails", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		err := newAdd(store, &bytes.Buffer{}).run(t.Context(), addOptions{name: "/app/config"})
		require.ErrorIs(t, err, valueinput.ErrValueRequired)

		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("edit without value on a non-TTY stdin fails", func(t *testing.T) {
		t.Parallel()

		// The check runs before the remote fetch, so a fetch failure (e.g. no
		// credentials in CI) does not hide it.
		r := newEdit(testutil.NewMockStore(), &bytes.Buffer{})
		r.useCase.Strategy = &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("no credentials")}
		err := r.run(t.Context(), editOptions{name: "/app/config"})
		require.ErrorIs(t, err, valueinput.ErrValueRequired)
	})

	t.Run("add with an explicit empty value stages it", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		require.NoError(t, newAdd(store, &bytes.Buffer{}).run(t.Context(), addOptions{name: "/app/config", hasValue: true}))
		assert.Empty(t, stagedValue(t, store))
	})

	t.Run("add --value-stdin", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		err := newAdd(store, bytes.NewBufferString("piped-value\n")).run(t.Context(), addOptions{
			name: "/app/config", valueFromStdin: true,
		})
		require.NoError(t, err)
		assert.Equal(t, "piped-value", stagedValue(t, store))
	})

	t.Run("edit --value-stdin", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		err := newEdit(store, bytes.NewBufferString("piped-value\n")).run(t.Context(), editOptions{
			name: "/app/config", valueFromStdin: true,
		})
		require.NoError(t, err)
		assert.Equal(t, "piped-value", stagedValue(t, store))
	})

	t.Run("--value-stdin with a value argument fails", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		err := newAdd(store, bytes.NewBufferString("piped-value")).run(t.Context(), addOptions{
			name: "/app/config", value: "arg", hasValue: true, valueFromStdin: true,
		})
		require.ErrorContains(t, err, "cannot combine a positional value with --value-stdin")
	})
}

// TestAddEditCommand_ExplicitEmptyValue drives stage add/edit through their
// commands: an explicit "" argument is an empty value, not a request for the
// editor, so it stages without a terminal (#985).
//
//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestAddEditCommand_ExplicitEmptyValue(t *testing.T) {
	scope := setupExportImportEnv(t)
	cfg := CommandConfig{
		CommandName:   "param",
		ItemName:      "parameter",
		CommandPath:   "suve stage param",
		ScopeResolver: exportFixedResolver(scope),
		Factory: func(context.Context) (staging.FullStrategy, error) {
			return &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "remote-value"}, nil
		},
	}

	stagedValue := func(t *testing.T, name string) string {
		t.Helper()

		entry, ok := exportWorkingState(t, scope).Entries[staging.ServiceParam][staging.EntryKey{Name: name}]
		require.True(t, ok, "%s must be staged", name)

		return lo.FromPtr(entry.Value)
	}

	addCfg := cfg
	addCfg.Factory = func(context.Context) (staging.FullStrategy, error) {
		return &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: &staging.ResourceNotFoundError{Err: errors.New("not found")}}, nil
	}

	stdout, _, err := exportRunLeafCmd(t, NewAddCommand(addCfg), &bytes.Buffer{}, "/app/new", "")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Staged for creation")
	assert.Empty(t, stagedValue(t, "/app/new"))

	stdout, _, err = exportRunLeafCmd(t, NewEditCommand(cfg), &bytes.Buffer{}, "/app/existing", "")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Staged")
	assert.Empty(t, stagedValue(t, "/app/existing"))

	_, _, err = exportRunLeafCmd(t, NewEditCommand(cfg), &bytes.Buffer{}, "/app/other")
	require.ErrorIs(t, err, valueinput.ErrValueRequired)
}
