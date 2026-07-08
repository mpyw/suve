package param

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{name: "no truncation when under limit", in: "hello", maxLen: 10, want: "hello"},
		{name: "no truncation at exact limit", in: "hello", maxLen: 5, want: "hello"},
		{name: "ascii truncated with ellipsis", in: "hello world", maxLen: 5, want: "hello..."},
		{name: "zero maxLen disables truncation", in: "hello", maxLen: 0, want: "hello"},
		{name: "negative maxLen disables truncation", in: "hello", maxLen: -1, want: "hello"},
		// Byte slicing would cut a 3-byte rune apart here; rune slicing keeps
		// the first maxLen characters whole (#340).
		{name: "multibyte kept whole", in: "日本語テスト", maxLen: 3, want: "日本語..."},
		{name: "emoji kept whole", in: "🎉🎊🎈🎆", maxLen: 2, want: "🎉🎊..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncateRunes(tt.in, tt.maxLen)
			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got), "result must be valid UTF-8")
		})
	}
}

func TestSanitizeControl(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a␤b", sanitizeControl("a\nb"))
	assert.Equal(t, "a␤b␤c", sanitizeControl("a\nb\tc"))
	// Each control char maps individually, so CRLF becomes two markers.
	assert.Equal(t, "a␤␤b", sanitizeControl("a\r\nb"))
	assert.Equal(t, "plain text", sanitizeControl("plain text"))
	// Multi-byte, non-control content is left untouched.
	assert.Equal(t, "日本語", sanitizeControl("日本語"))
}
