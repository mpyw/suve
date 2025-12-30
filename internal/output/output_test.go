package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriter_Field(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf)

	w.Field("Name", "test-value")

	output := buf.String()
	if !strings.Contains(output, "Name:") {
		t.Errorf("expected output to contain 'Name:', got %s", output)
	}
	if !strings.Contains(output, "test-value") {
		t.Errorf("expected output to contain 'test-value', got %s", output)
	}
}

func TestWriter_Separator(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf)

	w.Separator()

	output := buf.String()
	if output != "\n" {
		t.Errorf("expected newline, got %q", output)
	}
}

func TestWriter_Value(t *testing.T) {
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
			var buf bytes.Buffer
			w := New(&buf)

			w.Value(tt.value)

			output := buf.String()
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q, got %q", expected, output)
				}
			}
		})
	}
}

func TestWriter_ValuePreview(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		maxLen   int
		contains string
	}{
		{
			name:     "short value",
			value:    "short",
			maxLen:   10,
			contains: "short",
		},
		{
			name:     "truncated value",
			value:    "this is a very long value",
			maxLen:   10,
			contains: "this is a ...",
		},
		{
			name:     "newlines replaced",
			value:    "line1\nline2",
			maxLen:   20,
			contains: "line1 line2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := New(&buf)

			w.ValuePreview(tt.value, tt.maxLen)

			output := buf.String()
			if !strings.Contains(output, tt.contains) {
				t.Errorf("expected output to contain %q, got %q", tt.contains, output)
			}
		})
	}
}

func TestDiff(t *testing.T) {
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
			result := Diff(tt.oldName, tt.newName, tt.oldContent, tt.newContent)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("expected diff to contain %q, got:\n%s", expected, result)
				}
			}

			for _, notExpected := range tt.notContain {
				if strings.Contains(result, notExpected) {
					t.Errorf("expected diff NOT to contain %q, got:\n%s", notExpected, result)
				}
			}
		})
	}
}

func TestDiff_EmptyInputs(t *testing.T) {
	result := Diff("old", "new", "", "")
	if result != "" {
		t.Errorf("expected empty diff for identical empty strings, got %q", result)
	}
}

func TestColorDiff(t *testing.T) {
	diff := "--- old\n+++ new\n@@ -1 +1 @@\n-removed\n+added\n context"

	result := colorDiff(diff)

	// Just verify it doesn't panic and returns something
	if result == "" {
		t.Error("expected non-empty result")
	}

	// The output should still contain the original content (possibly with ANSI codes)
	if !strings.Contains(result, "removed") {
		t.Error("expected result to contain 'removed'")
	}
	if !strings.Contains(result, "added") {
		t.Error("expected result to contain 'added'")
	}
}

func TestColorDiff_EmptyInput(t *testing.T) {
	result := colorDiff("")
	if result != "" {
		t.Errorf("expected empty result for empty input, got %q", result)
	}
}
