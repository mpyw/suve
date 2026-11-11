//go:build production || dev

// The native file pickers behind the staging export and import bindings.
// stagingWailsFileDialogs needs a running Wails application, so it is
// untestable glue (excluded in codecov.yml); the bindings build the dialog
// options and tests check them through a fake stagingFileDialogs.
//
// The staging*.go files are one set of App methods split by concern, so they
// share staging.go's namespace.
//declscope:namespace staging

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// stagingFileDialogs shows a native file dialog and returns the chosen path, or ""
// with no error when the user cancels.
//
//declscope:shared // the App field in app.go holds one
type stagingFileDialogs interface {
	SaveFile(options *application.SaveFileDialogOptions) (string, error)
	OpenFile(options *application.OpenFileDialogOptions) (string, error)
}

// stagingWailsFileDialogs shows the dialogs through the running Wails application.
//
//declscope:shared // newApp in app.go installs it
type stagingWailsFileDialogs struct{}

// SaveFile shows the native Save dialog.
func (stagingWailsFileDialogs) SaveFile(options *application.SaveFileDialogOptions) (string, error) {
	return application.Get().Dialog.SaveFileWithOptions(options).PromptForSingleSelection()
}

// OpenFile shows the native Open dialog for a single file.
func (stagingWailsFileDialogs) OpenFile(options *application.OpenFileDialogOptions) (string, error) {
	return application.Get().Dialog.OpenFileWithOptions(options).PromptForSingleSelection()
}
