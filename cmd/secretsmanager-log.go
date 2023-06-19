package main

import (
	"fmt"

	"github.com/mpyw/suve/internal/typeconv"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/actions/log"
	"github.com/mpyw/suve/pkg/core/versioning"
	"github.com/urfave/cli/v2"
)

var SecretsManagerLogCommand = &cli.Command{
	Name:      "log",
	Usage:     "Show secret version changes",
	ArgsUsage: "<secret-id> [start-version-id|start-version-stage]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "pretty-json",
			Aliases: []string{"j"},
			Value:   false,
			Usage:   "Pretty printing for JSON (keys are automatically sorted)",
		},
		&cli.IntFlag{
			Name:  "max-results",
			Value: 5,
			Usage: "Number of versions to show",
		},
		&cli.IntFlag{
			Name:  "max-results-to-search",
			Value: 15,
			Usage: "Number of versions to search",
		},
	},
	Action: withSecretsManagerAlwaysIncludeDeprecated(func(c *cli.Context) error {
		deps := actions.GetDependencies(c.Context)
		secretID := c.Args().First()
		if secretID == "" {
			return fmt.Errorf("secret-id is required")
		}
		var version *versioning.VersionRequirement
		if str := c.Args().Get(1); str != "" {
			v, err := deps.VersionParser.Parse(str)
			if err != nil {
				return err
			}
			version = &v
		}
		maxResultsToSearch := c.Int("max-results-to-search")
		if maxResultsToSearch < 1 {
			return fmt.Errorf("max-results-to-search must be greater than or equal to 1")
		}
		maxResults := c.Int("max-results")
		if maxResults < 1 {
			return fmt.Errorf("max-results must be greater than or equal to 1")
		}
		return log.Action(c.Context, log.ActionInput{
			Dependencies:       deps,
			Name:               secretID,
			PrettyJSON:         c.Bool("pretty-json"),
			Version:            version,
			MaxResults:         typeconv.Ref(int32(maxResults)),
			MaxResultsToSearch: typeconv.Ref(int32(maxResultsToSearch)),
		})
	}),
}

func init() {
	SecretsManagerCommand.Subcommands = append(SecretsManagerCommand.Subcommands, SecretsManagerLogCommand)
}
