// White-box tests of paramtype.go.
//declscope:namespace paramtype

package paramtype

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		{"String", String, false},
		{"SecureString", SecureString, false},
		{"StringList", stringList, false},
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

			err := Validate(tt.input)
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

	assert.Equal(t, domain.ValueTypeSecret, Parse(SecureString))
	assert.Equal(t, domain.ValueTypeList, Parse(stringList))
	assert.Equal(t, domain.ValueTypePlaintext, Parse(String))
	assert.Equal(t, domain.ValueTypePlaintext, Parse(""))
}
