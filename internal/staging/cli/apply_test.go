package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging/cli"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestFormatTagApplySummary(t *testing.T) {
	t.Parallel()

	t.Run("add only", func(t *testing.T) {
		t.Parallel()

		result := stagingusecase.ApplyTagResult{
			AddTags:   map[string]string{"env": "prod", "team": "backend"},
			RemoveTag: maputil.NewSet[string](),
		}
		summary := cli.FormatTagApplySummary(result)
		assert.Equal(t, " [+2]", summary)
	})

	t.Run("remove only", func(t *testing.T) {
		t.Parallel()

		result := stagingusecase.ApplyTagResult{
			AddTags:   map[string]string{},
			RemoveTag: maputil.NewSet("deprecated", "old", "obsolete"),
		}
		summary := cli.FormatTagApplySummary(result)
		assert.Equal(t, " [-3]", summary)
	})

	t.Run("add and remove", func(t *testing.T) {
		t.Parallel()

		result := stagingusecase.ApplyTagResult{
			AddTags:   map[string]string{"env": "prod"},
			RemoveTag: maputil.NewSet("deprecated", "old"),
		}
		summary := cli.FormatTagApplySummary(result)
		assert.Equal(t, " [+1, -2]", summary)
	})

	t.Run("empty - unreachable branch", func(t *testing.T) {
		t.Parallel()

		result := stagingusecase.ApplyTagResult{
			AddTags:   map[string]string{},
			RemoveTag: maputil.NewSet[string](),
		}
		summary := cli.FormatTagApplySummary(result)
		assert.Empty(t, summary)
	})
}
