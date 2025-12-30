package jsonutil

import (
	"testing"
)

func TestFormat(t *testing.T) {
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
			got := Format(tt.input)
			if got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}
