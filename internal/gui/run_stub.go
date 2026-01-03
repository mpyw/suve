//go:build !desktop

package gui

import "errors"

// Run returns an error when GUI is not available in this build.
func Run() error {
	return errors.New("GUI is not available in this build. Please use a desktop build or run from the gui/ directory with 'wails dev'")
}
