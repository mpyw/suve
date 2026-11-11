//go:build production || dev

// In-package tests of the staging file-picker bindings, through a fake
// stagingFileDialogs.
//declscope:namespace staging

package gui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/mpyw/suve/internal/provider"
)

// stagingFakeFileDialogs records the options each dialog was shown with and returns
// a fixed path and error.
type stagingFakeFileDialogs struct {
	path string
	err  error
	save *application.SaveFileDialogOptions
	open *application.OpenFileDialogOptions
}

func (f *stagingFakeFileDialogs) SaveFile(options *application.SaveFileDialogOptions) (string, error) {
	f.save = options

	return f.path, f.err
}

func (f *stagingFakeFileDialogs) OpenFile(options *application.OpenFileDialogOptions) (string, error) {
	f.open = options

	return f.path, f.err
}

func TestNewApp_InstallsStagingWailsFileDialogs(t *testing.T) {
	t.Parallel()

	app := newTestApp(t, provider.Scope{Provider: provider.ProviderAWS}, "")
	assert.IsType(t, stagingWailsFileDialogs{}, app.dialogs)
}

func TestStagingPickExportPath(t *testing.T) {
	t.Parallel()

	dialogs := &stagingFakeFileDialogs{path: "/tmp/out.json"}
	app := &App{dialogs: dialogs}

	path, err := app.StagingPickExportPath("suve-param.json")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/out.json", path)

	require.NotNil(t, dialogs.save)
	assert.Equal(t, "Export staged changes", dialogs.save.Title)
	assert.Equal(t, "suve-param.json", dialogs.save.Filename)
	assert.Equal(t, []application.FileFilter{{DisplayName: "JSON files (*.json)", Pattern: "*.json"}}, dialogs.save.Filters)
}

func TestStagingPickImportPath(t *testing.T) {
	t.Parallel()

	dialogs := &stagingFakeFileDialogs{path: "/tmp/in.json"}
	app := &App{dialogs: dialogs}

	path, err := app.StagingPickImportPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/in.json", path)

	require.NotNil(t, dialogs.open)
	assert.Equal(t, "Import staged changes", dialogs.open.Title)
	// Without CanChooseFiles, Wails v3 shows a dialog that selects nothing.
	assert.True(t, dialogs.open.CanChooseFiles)
	assert.Equal(t, []application.FileFilter{{DisplayName: "JSON files (*.json)", Pattern: "*.json"}}, dialogs.open.Filters)
}

func TestStagingPickPaths_PassDialogResultThrough(t *testing.T) {
	t.Parallel()

	errDialog := errors.New("dialog failed")

	tests := []struct {
		name string
		pick func(*App) (string, error)
	}{
		{name: "export", pick: func(a *App) (string, error) { return a.StagingPickExportPath("x.json") }},
		{name: "import", pick: func(a *App) (string, error) { return a.StagingPickImportPath() }},
	}

	for _, tt := range tests {
		t.Run(tt.name+" cancelled", func(t *testing.T) {
			t.Parallel()

			path, err := tt.pick(&App{dialogs: &stagingFakeFileDialogs{}})
			require.NoError(t, err)
			assert.Empty(t, path, "a cancelled dialog is an empty path with no error")
		})

		t.Run(tt.name+" error", func(t *testing.T) {
			t.Parallel()

			_, err := tt.pick(&App{dialogs: &stagingFakeFileDialogs{err: errDialog}})
			require.ErrorIs(t, err, errDialog)
		})
	}
}
