package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
