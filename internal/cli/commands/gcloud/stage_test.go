package gcloud_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/gcloud"
)

// TestGoogleCloudStageHelpWording verifies the stage help text names Google Cloud and uses the explicit
// "suve gcloud stage" command paths, never another provider's wording or the flat
// alias paths.
func TestGoogleCloudStageHelpWording(t *testing.T) {
	t.Parallel()

	for path, text := range gcloudStageHelpTexts(gcloud.StageCommand(), "suve gcloud stage") {
		assert.NotContains(t, text, "AWS", path)
		for _, m := range regexp.MustCompile(`suve (?:aws|gcloud|azure|param|secret|stage|stg)\b`).FindAllString(text, -1) {
			assert.Equal(t, "suve gcloud", m, "%s: %q", path, text)
		}
	}
}

// gcloudStageHelpTexts returns every help string (usage, description, flag usages) in the
// command tree, keyed by the command path.
func gcloudStageHelpTexts(cmd *cli.Command, path string) map[string]string {
	texts := map[string]string{}

	var walk func(c *cli.Command, p string)
	walk = func(c *cli.Command, p string) {
		parts := []string{c.Usage, c.Description}
		for _, f := range c.Flags {
			if u, ok := f.(interface{ GetUsage() string }); ok {
				parts = append(parts, u.GetUsage())
			}
		}

		texts[p] = strings.Join(parts, "\n")

		for _, sub := range c.Commands {
			walk(sub, p+" "+sub.Name)
		}
	}
	walk(cmd, path)

	return texts
}
