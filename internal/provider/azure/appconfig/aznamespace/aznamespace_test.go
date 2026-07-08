package aznamespace_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty maps to null-label filter", raw: "", want: "\x00"},
		{name: "wildcard all forwarded", raw: "*", want: "*"},
		{name: "OR-list forwarded", raw: "dev,prod", want: "dev,prod"},
		{name: "prefix wildcard forwarded", raw: "dev*", want: "dev*"},
		{name: "literal forwarded", raw: "dev", want: "dev"},
		{name: "escaped reserved forwarded verbatim", raw: `\*`, want: `\*`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, aznamespace.Filter(tt.raw))
		})
	}
}

func TestLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty maps to null namespace", raw: "", want: ""},
		{name: "plain literal", raw: "dev", want: "dev"},
		{name: "escaped star decodes to literal star", raw: `\*`, want: "*"},
		{name: "escaped comma decodes to literal comma", raw: `foo\,bar`, want: "foo,bar"},
		{name: "escaped backslash decodes to single backslash", raw: `\\`, want: `\`},
		{name: "trailing backslash kept literal", raw: `foo\`, want: `foo\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := aznamespace.Literal(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLiteral_UnescapedFilterCharsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "wildcard all", raw: "*"},
		{name: "OR-list", raw: "dev,prod"},
		{name: "prefix wildcard", raw: "dev*"},
		{name: "comma after escaped star", raw: `\*,x`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := aznamespace.Literal(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "single-item operation needs one")
		})
	}
}
