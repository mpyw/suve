// Package editor provides functionality for opening external editors.
package editor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/samber/lo"
)

// OpenFunc is the type for editor functions.
type OpenFunc func(ctx context.Context, content string) (string, error)

// Open opens the content in an external editor and returns the edited result.
// It uses the VISUAL or EDITOR environment variable to determine the editor.
// Falls back to notepad on Windows or vi on other platforms.
func Open(ctx context.Context, content string) (string, error) {
	editor := lo.CoalesceOrEmpty(
		os.Getenv("VISUAL"),
		os.Getenv("EDITOR"),
		lo.Ternary(runtime.GOOS == "windows", "notepad", "vi"),
	)

	tmpFile, err := os.CreateTemp("", "suve-edit-*.txt")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", errors.Join(err, tmpFile.Close())
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	// Split editor command to support multi-argument editors like "code --wait"
	args := strings.Fields(editor)
	args = append(args, tmpFile.Name())
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // Editor command from user environment
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
	// Handle both Unix (\n) and Windows (\r\n) line endings
	result := string(data)
	result = strings.TrimSuffix(result, "\r\n")
	result = strings.TrimSuffix(result, "\n")

	return result, nil
}
