package paramopts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/param/paramopts"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
)

func TestValidateTier(t *testing.T) {
	t.Parallel()

	for _, tier := range []string{"", "Standard", "Advanced", "Intelligent-Tiering"} {
		require.NoError(t, paramopts.ValidateTier(tier))
	}

	err := paramopts.ValidateTier("Bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --tier")
}

func TestBuild(t *testing.T) {
	t.Parallel()

	t.Run("empty values yield no options", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, paramopts.Build(paramopts.Values{}))
	})

	t.Run("set values map to typed options", func(t *testing.T) {
		t.Parallel()

		opts := paramopts.Build(paramopts.Values{
			Tier:           "Advanced",
			DataType:       "text",
			AllowedPattern: "^a",
			Policies:       "[]",
		})

		require.Len(t, opts, 4)
		assert.Contains(t, opts, awsparam.Tier{Value: "Advanced"})
		assert.Contains(t, opts, awsparam.DataType{Value: "text"})
		assert.Contains(t, opts, awsparam.AllowedPattern{Value: "^a"})
		assert.Contains(t, opts, awsparam.Policies{JSON: "[]"})
	})
}
