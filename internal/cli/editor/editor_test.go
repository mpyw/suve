package editor_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/editor"
)

const goosWindows = "windows"

func TestOpen_ModifiesContent(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Skipping on Windows - requires Unix shell")
	}

	// Create a test script that appends "-modified" to the file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-editor.sh")
	script := `#!/bin/sh
content=$(cat "$1")
printf '%s-modified' "$content" > "$1"
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	// Set EDITOR to our test script
	t.Setenv("EDITOR", scriptPath)
	t.Setenv("VISUAL", "")

	result, err := editor.Open("original")
	require.NoError(t, err)
	assert.Equal(t, "original-modified", result)
}

func TestOpen_ReturnsUnmodifiedContent(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Skipping on Windows - requires Unix shell")
	}

	// Use 'true' as editor (no-op command)
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "")

	result, err := editor.Open("unchanged content")
	require.NoError(t, err)
	assert.Equal(t, "unchanged content", result)
}

func TestOpen_TrimsTrailingNewline(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Skipping on Windows - requires Unix shell")
	}

	// Create a script that adds a trailing newline
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-editor.sh")
	script := `#!/bin/sh
echo "with-newline" > "$1"
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("EDITOR", scriptPath)
	t.Setenv("VISUAL", "")

	result, err := editor.Open("original")
	require.NoError(t, err)
	assert.Equal(t, "with-newline", result)
}

func TestOpen_TrimsCRLF(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Skipping on Windows - requires Unix shell")
	}

	// Create a script that adds CRLF
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-editor.sh")
	script := `#!/bin/sh
printf "with-crlf\r\n" > "$1"
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	t.Setenv("EDITOR", scriptPath)
	t.Setenv("VISUAL", "")

	result, err := editor.Open("original")
	require.NoError(t, err)
	assert.Equal(t, "with-crlf", result)
}

func TestOpen_UsesVISUALOverEDITOR(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Skipping on Windows - requires Unix shell")
	}

	// Create two different scripts
	tmpDir := t.TempDir()
	visualScript := filepath.Join(tmpDir, "visual.sh")
	editorScript := filepath.Join(tmpDir, "editor.sh")

	require.NoError(t, os.WriteFile(visualScript, []byte(`#!/bin/sh
printf 'visual' > "$1"
`), 0o755))

	require.NoError(t, os.WriteFile(editorScript, []byte(`#!/bin/sh
printf 'editor' > "$1"
`), 0o755))

	// VISUAL should take precedence
	t.Setenv("VISUAL", visualScript)
	t.Setenv("EDITOR", editorScript)

	result, err := editor.Open("original")
	require.NoError(t, err)
	assert.Equal(t, "visual", result)
}

func TestOpen_EditorError(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("Skipping on Windows - requires Unix shell")
	}

	// Set editor to a command that exits with error
	t.Setenv("EDITOR", "false")
	t.Setenv("VISUAL", "")

	_, err := editor.Open("content")
	require.Error(t, err)
}
