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

// Command builds the *exec.Cmd that opens tmpFile in the user's editor. It
// resolves VISUAL then EDITOR, falling back to notepad on Windows or vi on other
// platforms, and invokes through the shell (`sh -c 'editor "$1"' sh tmpFile`) so
// that quoting and arguments behave exactly like Git does: the temp-file path is
// passed as $1 so an $EDITOR that itself carries flags or a space-containing path
// is expanded by the shell (e.g. "code --wait" or
// "/Applications/Visual Studio Code.app/.../code").
//
// It does NOT wire Stdin/Stdout/Stderr, so the caller (the CLI, or the TUI via
// tea.ExecProcess) owns the terminal handoff. This is the single command builder
// both the CLI editor and the TUI's $EDITOR handoff share, so they cannot
// diverge on VISUAL→EDITOR precedence, shell quoting, or the OS fallback.
func Command(ctx context.Context, tmpFile string) *exec.Cmd {
	editor := lo.CoalesceOrEmpty(
		os.Getenv("VISUAL"),
		os.Getenv("EDITOR"),
		lo.Ternary(runtime.GOOS == "windows", "notepad", "vi"),
	)

	//nolint:gosec // $VISUAL/$EDITOR are user-controlled by design
	return exec.CommandContext(ctx, "sh", "-c", editor+` "$1"`, "sh", tmpFile)
}

// Open opens the content in an external editor and returns the edited result.
// It uses the VISUAL or EDITOR environment variable to determine the editor.
// Falls back to notepad on Windows or vi on other platforms.
func Open(ctx context.Context, content string) (string, error) {
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

	cmd := Command(ctx, tmpFile.Name())
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

	return Normalize(content, string(data)), nil
}

// Normalize applies the round-trip-lossless rule to the editor's read-back
// (result) given the exact content that was written into the tmpfile.
//
// Editors typically auto-append a single trailing newline to a buffer that did
// not already end with one. We reverse ONLY that editor-added newline: if the
// original content did not end with a newline, a single trailing newline (Unix
// "\n" or Windows "\r\n") is stripped from the read-back. A trailing newline the
// value carried in (PEM keys, JSON, ...) is preserved, so opening a value and
// saving it untouched is byte-identical — which the callers then detect and
// report as a no-op change.
func Normalize(content, result string) string {
	// The value already carried a trailing newline: the editor did not add one,
	// so keep the read-back verbatim to stay lossless.
	if strings.HasSuffix(content, "\n") {
		return result
	}

	// Strip the single line ending the editor auto-appended, handling both Unix
	// (\n) and Windows (\r\n) endings.
	if trimmed, ok := strings.CutSuffix(result, "\r\n"); ok {
		return trimmed
	}

	return strings.TrimSuffix(result, "\n")
}
