// White-box tests for writeoption.go's tier validation and option building.
//declscope:namespace writeoption

package param

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/aws/parameterstore"
)

func TestValidateWriteOptionTier(t *testing.T) {
	t.Parallel()

	for _, tier := range []string{"", "Standard", "Advanced", "Intelligent-Tiering"} {
		require.NoError(t, validateWriteOptionTier(tier))
	}

	err := validateWriteOptionTier("Bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --tier")
}

func TestBuildWriteOptions(t *testing.T) {
	t.Parallel()

	t.Run("empty values yield no options", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, buildWriteOptions(WriteOptionFlags{}))
	})

	t.Run("set values map to typed options", func(t *testing.T) {
		t.Parallel()

		opts := buildWriteOptions(WriteOptionFlags{
			Tier:           "Advanced",
			DataType:       "text",
			AllowedPattern: "^a",
			Policies:       "[]",
		})

		require.Len(t, opts, 4)
		assert.Contains(t, opts, parameterstore.Tier{Value: "Advanced"})
		assert.Contains(t, opts, parameterstore.DataType{Value: "text"})
		assert.Contains(t, opts, parameterstore.AllowedPattern{Value: "^a"})
		assert.Contains(t, opts, parameterstore.Policies{JSON: "[]"})
	})
}
