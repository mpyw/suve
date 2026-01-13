package jsonutil_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/jsonutil"
)

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

func TestTryFormatOrWarn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		itemName    string
		wantWarning bool
	}{
		{
			name:        "valid json no warning",
			value:       `{"key":"value"}`,
			wantWarning: false,
		},
		{
			name:        "invalid json warns",
			value:       "not json",
			wantWarning: true,
		},
		{
			name:        "invalid json with name",
			value:       "not json",
			itemName:    "my-secret",
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var errBuf bytes.Buffer

			result := jsonutil.TryFormatOrWarn(tt.value, &errBuf, tt.itemName)

			if tt.wantWarning {
				assert.Contains(t, errBuf.String(), "Warning:")
				assert.Contains(t, errBuf.String(), "--parse-json has no effect")

				if tt.itemName != "" {
					assert.Contains(t, errBuf.String(), tt.itemName)
				}

				assert.Equal(t, tt.value, result)
			} else {
				assert.Empty(t, errBuf.String())
			}
		})
	}
}

func TestTryFormatOrWarn2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		v1          string
		v2          string
		itemName    string
		wantWarning bool
	}{
		{
			name:        "both valid json",
			v1:          `{"a":1}`,
			v2:          `{"b":2}`,
			wantWarning: false,
		},
		{
			name:        "one invalid warns",
			v1:          `{"a":1}`,
			v2:          "not json",
			wantWarning: true,
		},
		{
			name:        "both invalid warns",
			v1:          "not json",
			v2:          "also not json",
			wantWarning: true,
		},
		{
			name:        "with name in warning",
			v1:          "not json",
			v2:          `{"b":2}`,
			itemName:    "my-param",
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var errBuf bytes.Buffer

			r1, r2 := jsonutil.TryFormatOrWarn2(tt.v1, tt.v2, &errBuf, tt.itemName)

			if tt.wantWarning {
				assert.Contains(t, errBuf.String(), "Warning:")
				assert.Contains(t, errBuf.String(), "--parse-json has no effect")

				if tt.itemName != "" {
					assert.Contains(t, errBuf.String(), tt.itemName)
				}

				assert.Equal(t, tt.v1, r1)
				assert.Equal(t, tt.v2, r2)
			} else {
				assert.Empty(t, errBuf.String())
			}
		})
	}
}
