package jsonutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/jsonutil"
)

func TestFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple object",
			input: `{"key":"value"}`,
			want:  "{\n  \"key\": \"value\"\n}",
		},
		{
			name:  "nested object",
			input: `{"outer":{"inner":"value"}}`,
			want:  "{\n  \"outer\": {\n    \"inner\": \"value\"\n  }\n}",
		},
		{
			name:  "array",
			input: `["a","b","c"]`,
			want:  "[\n  \"a\",\n  \"b\",\n  \"c\"\n]",
		},
		{
			name:  "complex structure",
			input: `{"name":"test","values":[1,2,3],"nested":{"key":"value"}}`,
			want:  "{\n  \"name\": \"test\",\n  \"nested\": {\n    \"key\": \"value\"\n  },\n  \"values\": [\n    1,\n    2,\n    3\n  ]\n}",
		},
		{
			name:  "already formatted",
			input: "{\n  \"key\": \"value\"\n}",
			want:  "{\n  \"key\": \"value\"\n}",
		},
		{
			name:  "invalid json returns original",
			input: "not json",
			want:  "not json",
		},
		{
			name:  "empty string returns original",
			input: "",
			want:  "",
		},
		{
			name:  "plain string returns original",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "number as json",
			input: "123",
			want:  "123",
		},
		{
			name:  "boolean as json",
			input: "true",
			want:  "true",
		},
		{
			name:  "null as json",
			input: "null",
			want:  "null",
		},
		{
			name:  "string as json",
			input: `"hello"`,
			want:  `"hello"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := jsonutil.Format(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid object", input: `{"key":"value"}`, want: true},
		{name: "valid array", input: `[1,2,3]`, want: true},
		{name: "valid string", input: `"hello"`, want: true},
		{name: "valid number", input: `123`, want: true},
		{name: "valid boolean", input: `true`, want: true},
		{name: "valid null", input: `null`, want: true},
		{name: "invalid json", input: `not json`, want: false},
		{name: "empty string", input: ``, want: false},
		{name: "incomplete object", input: `{"key":`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := jsonutil.IsJSON(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTryFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantStr  string
		wantBool bool
	}{
		{
			name:     "valid object formats and returns true",
			input:    `{"key":"value"}`,
			wantStr:  "{\n  \"key\": \"value\"\n}",
			wantBool: true,
		},
		{
			name:     "invalid json returns original and false",
			input:    "not json",
			wantStr:  "not json",
			wantBool: false,
		},
		{
			name:     "empty string returns original and false",
			input:    "",
			wantStr:  "",
			wantBool: false,
		},
		{
			name:     "valid array formats and returns true",
			input:    `[1,2,3]`,
			wantStr:  "[\n  1,\n  2,\n  3\n]",
			wantBool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStr, gotBool := jsonutil.TryFormat(tt.input)
			assert.Equal(t, tt.wantStr, gotStr)
			assert.Equal(t, tt.wantBool, gotBool)
		})
	}
}
