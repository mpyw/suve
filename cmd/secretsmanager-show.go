package main

import (
	"fmt"

	"github.com/mpyw/suve/internal/typeconv"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/actions/show"
	"github.com/mpyw/suve/pkg/core/versioning"
	"github.com/urfave/cli/v2"
)

var SecretsManagerShowCommand = &cli.Command{
	Name:      "show",
	Usage:     "Show a specific version of a secret",
	ArgsUsage: "<secret-id> [version-id|version-stage]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "raw",
			Aliases: []string{"r"},
			Value:   false,
			Usage:   "Output without any headers",
		},
		&cli.BoolFlag{
			Name:    "pretty-json",
			Aliases: []string{"j"},
			Value:   false,
			Usage:   "Pretty printing for JSON (keys are automatically sorted)",
		},
		&cli.BoolFlag{
			Name:  "include-deprecated",
			Value: false,
			Usage: "Include deprecated (pending to be deleted) secret",
		},
		&cli.IntFlag{
			Name:  "max-results-to-search",
			Value: 15,
			Usage: "Number of versions to search",
		},
	},
	Action: withSecretsManager(func(c *cli.Context) error {
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
		return show.Action(c.Context, show.ActionInput{
			Dependencies:       deps,
			Name:               secretID,
			PrettyJSON:         c.Bool("pretty-json"),
			Raw:                c.Bool("raw"),
			Version:            version,
			MaxResultsToSearch: typeconv.Ref(int32(maxResultsToSearch)),
		})
	}),
}

func init() {
	SecretsManagerCommand.Subcommands = append(SecretsManagerCommand.Subcommands, SecretsManagerShowCommand)
}
