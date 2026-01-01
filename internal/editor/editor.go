// Package editor provides functionality for opening external editors.
package editor

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenFunc is the type for editor functions.
type OpenFunc func(content string) (string, error)

// Open opens the content in an external editor and returns the edited result.
// It uses the VISUAL or EDITOR environment variable to determine the editor.
// Falls back to notepad on Windows or vi on other platforms.
func Open(content string) (string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}

	tmpFile, err := os.CreateTemp("", "suve-edit-*.txt")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	cmd := exec.Command(editor, tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return "", err
	}

	// Trim trailing newline that editors often add
	result := string(data)
	result = strings.TrimSuffix(result, "\n")

	return result, nil
}
