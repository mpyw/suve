package paramtype_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/param/paramtype"
	"github.com/mpyw/suve/internal/domain"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty means default (String)", "", false},
		{"String", paramtype.String, false},
		{"SecureString", paramtype.SecureString, false},
		{"StringList", paramtype.StringList, false},
		// The bug this guards: a typo/wrong-case must NOT silently fall through to
		// plaintext — it must be rejected so an intended-encrypted value is never
		// stored as a plain String.
		{"lowercase securestring rejected", "securestring", true},
		{"typo SecureSting rejected", "SecureSting", true},
		{"garbage rejected", "Nope", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := paramtype.Validate(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid --type")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.ValueTypeSecret, paramtype.Parse(paramtype.SecureString))
	assert.Equal(t, domain.ValueTypeList, paramtype.Parse(paramtype.StringList))
	assert.Equal(t, domain.ValueTypePlaintext, paramtype.Parse(paramtype.String))
	assert.Equal(t, domain.ValueTypePlaintext, paramtype.Parse(""))
}
