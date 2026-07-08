package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/terminal"
)

// fakeTTY is a bytes.Buffer that also satisfies terminal.Fder, so
// terminal.IsTerminalWriter can classify it as a terminal when IsTTY is mocked.
type fakeTTY struct {
	bytes.Buffer
}

func (f *fakeTTY) Fd() uintptr { return 1 }

func TestWriter_Field(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := New(&buf)

	w.Field("Name", "test-value")

	output := buf.String()
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "test-value")
}

func TestWriter_Separator(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := New(&buf)

	w.Separator()

	assert.Equal(t, "\n", buf.String())
}

func TestWriter_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		contains []string
	}{
		{
			name:     "single line",
			value:    "test-value",
			contains: []string{"  test-value"},
		},
		{
			name:     "multi line",
			value:    "line1\nline2\nline3",
			contains: []string{"  line1", "  line2", "  line3"},
		},
		{
			name:     "empty",
			value:    "",
			contains: []string{"  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			w := New(&buf)

			w.Value(tt.value)

			output := buf.String()
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		oldName    string
		newName    string
		oldContent string
		newContent string
		contains   []string
		notContain []string
	}{
		{
			name:       "no changes",
			oldName:    "file1",
			newName:    "file2",
			oldContent: "same content",
			newContent: "same content",
			notContain: []string{"-same", "+same"},
		},
		{
			name:       "added line",
			oldName:    "old",
			newName:    "new",
			oldContent: "line1",
			newContent: "line1\nline2",
			contains:   []string{"+line2"},
		},
		{
			name:       "removed line",
			oldName:    "old",
			newName:    "new",
			oldContent: "line1\nline2",
			newContent: "line1",
			contains:   []string{"-line2"},
		},
		{
			name:       "changed line",
			oldName:    "old",
			newName:    "new",
			oldContent: "old-value",
			newContent: "new-value",
			contains:   []string{"-old-value", "+new-value"},
		},
		{
			name:       "headers present",
			oldName:    "file-a",
			newName:    "file-b",
			oldContent: "a",
			newContent: "b",
			contains:   []string{"---", "+++"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Diff(tt.oldName, tt.newName, tt.oldContent, tt.newContent)

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}

			for _, notExpected := range tt.notContain {
				assert.NotContains(t, result, notExpected)
			}
		})
	}
}

func TestDiff_EmptyInputs(t *testing.T) {
	t.Parallel()

	result := Diff("old", "new", "", "")
	assert.Empty(t, result)
}

func TestColorDiff(t *testing.T) {
	t.Parallel()

	diff := "--- old\n+++ new\n@@ -1 +1 @@\n-removed\n+added\n context"

	result := colorDiff(diff)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "removed")
	assert.Contains(t, result, "added")
}

func TestColorDiff_EmptyInput(t *testing.T) {
	t.Parallel()

	result := colorDiff("")
	assert.Empty(t, result)
}

// TestDiff_MatchesDiffRawNewlines guards #338: with color disabled, Diff is
// structure-only, so it must equal DiffRaw with no spurious trailing newline.
//
//nolint:paralleltest // toggles the process-global color.NoColor
func TestDiff_MatchesDiffRawNewlines(t *testing.T) {
	orig := color.NoColor

	t.Cleanup(func() { color.NoColor = orig })

	color.NoColor = true

	got := Diff("f", "f", "a\nb\n", "a\nc\n")
	raw := DiffRaw("f", "f", "a\nb\n", "a\nc\n")

	assert.Equal(t, raw, got)
	assert.False(t, strings.HasSuffix(got, "\n\n"), "must not doubly-terminate the diff")
}

// TestColorDiff_InHunkDashLinesNotHeaders guards #339: inside a hunk, a
// removed/added line whose content starts with -- / ++ must be colored as
// removed/added, not misclassified as a ---/+++ header.
//
//nolint:paralleltest // toggles the process-global color.NoColor
func TestColorDiff_InHunkDashLinesNotHeaders(t *testing.T) {
	orig := color.NoColor

	t.Cleanup(func() { color.NoColor = orig })

	color.NoColor = false

	diff := "--- old\n+++ new\n@@ -1 +1 @@\n--- removed content\n+++ added content"

	result := colorDiff(diff)

	// The in-hunk lines are wrapped exactly as removed/added would wrap them.
	assert.Contains(t, result, colors.DiffRemoved("--- removed content"))
	assert.Contains(t, result, colors.DiffAdded("+++ added content"))
	// And the real file-label headers are still colored as headers.
	assert.Contains(t, result, colors.DiffHeader("--- old"))
	assert.Contains(t, result, colors.DiffHeader("+++ new"))
}

func TestWarning(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Warning(&buf, "test %s", "message")
	assert.Contains(t, buf.String(), "Warning:")
	assert.Contains(t, buf.String(), "test message")
}

func TestHint(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Hint(&buf, "try %s", "this")
	assert.Contains(t, buf.String(), "Hint:")
	assert.Contains(t, buf.String(), "try this")
}

func TestError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Error(&buf, "error %d", 42)
	assert.Contains(t, buf.String(), "Error:")
	assert.Contains(t, buf.String(), "error 42")
}

func TestSuccess(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Success(&buf, "Created parameter %s", "/app/config")
	assert.Contains(t, buf.String(), "✓")
	assert.Contains(t, buf.String(), "Created parameter /app/config")
}

func TestFailed(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Failed(&buf, "/app/config", assert.AnError)
	assert.Contains(t, buf.String(), "Failed")
	assert.Contains(t, buf.String(), "/app/config")
	assert.Contains(t, buf.String(), assert.AnError.Error())
}

func TestInfo(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Info(&buf, "No changes %s", "staged")
	assert.Contains(t, buf.String(), "No changes staged")
}

// TestFeedback_ColorTracksWriterTTY guards #341: a feedback message's color is
// decided by its own destination writer, not the process-global color.NoColor.
// A terminal writer receives ANSI even when the global would say otherwise, and
// a non-terminal writer (a plain bytes.Buffer with no Fd) stays clean.
func TestFeedback_ColorTracksWriterTTY(t *testing.T) {
	origIsTTY := terminal.IsTTY
	origNoColor := color.NoColor

	t.Cleanup(func() {
		terminal.IsTTY = origIsTTY
		color.NoColor = origNoColor
	})
	t.Setenv("NO_COLOR", "")

	// Force the global off (as `suve … | cat` would) to prove color no longer
	// depends on it, then treat the mocked fd as a terminal.
	color.NoColor = true
	terminal.IsTTY = func(uintptr) bool { return true }

	var tty fakeTTY

	Warning(&tty, "heads up")
	assert.Contains(t, tty.String(), "\x1b[", "a terminal writer must receive ANSI color")
	assert.Contains(t, tty.String(), "heads up")

	var buf bytes.Buffer

	Warning(&buf, "heads up")
	assert.NotContains(t, buf.String(), "\x1b[", "a non-terminal writer must stay plain")
}

// TestFeedback_NoColorEnvDisables guards #341: NO_COLOR disables coloring even
// for a terminal writer.
func TestFeedback_NoColorEnvDisables(t *testing.T) {
	origIsTTY := terminal.IsTTY

	t.Cleanup(func() { terminal.IsTTY = origIsTTY })
	t.Setenv("NO_COLOR", "1")

	terminal.IsTTY = func(uintptr) bool { return true }

	var tty fakeTTY

	Error(&tty, "boom")
	assert.NotContains(t, tty.String(), "\x1b[", "NO_COLOR must suppress ANSI even on a TTY")
	assert.Contains(t, tty.String(), "Error: boom")
}

func TestParseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected Format
		wantErr  bool
	}{
		{input: "json", expected: FormatJSON},
		{input: "text", expected: FormatText},
		{input: "", expected: FormatText},
		{input: "JSON", wantErr: true}, // not case-insensitive
		{input: "invalid", wantErr: true},
		{input: "jsonn", wantErr: true}, // the #349 typo
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			result, err := ParseFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--output")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDiffRaw(t *testing.T) {
	t.Parallel()

	result := DiffRaw("old", "new", "old-value", "new-value")
	assert.Contains(t, result, "--- old")
	assert.Contains(t, result, "+++ new")
	assert.Contains(t, result, "-old-value")
	assert.Contains(t, result, "+new-value")
}

func TestDiffRaw_EmptyInputs(t *testing.T) {
	t.Parallel()

	result := DiffRaw("old", "new", "", "")
	assert.Empty(t, result)
}

func TestIndent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "single line",
			input:    "hello",
			prefix:   "  ",
			expected: "  hello",
		},
		{
			name:     "multi line",
			input:    "line1\nline2\nline3",
			prefix:   "> ",
			expected: "> line1\n> line2\n> line3",
		},
		{
			name:     "empty string",
			input:    "",
			prefix:   "  ",
			expected: "",
		},
		{
			name:     "empty lines preserved",
			input:    "line1\n\nline3",
			prefix:   "  ",
			expected: "  line1\n\n  line3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Indent(tt.input, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWarn(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Warn(&buf, "warning %s", "message")
	assert.Contains(t, buf.String(), "!")
	assert.Contains(t, buf.String(), "warning message")
}

func TestPrint(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Print(&buf, "hello")
	assert.Equal(t, "hello", buf.String())
}

func TestPrintln(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Println(&buf, "hello")
	assert.Equal(t, "hello\n", buf.String())
}

func TestPrintf(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	Printf(&buf, "hello %s %d", "world", 42)
	assert.Equal(t, "hello world 42", buf.String())
}
