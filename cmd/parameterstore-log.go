package main

import (
	"fmt"

	"github.com/mpyw/suve/internal/typeconv"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/actions/log"
	"github.com/mpyw/suve/pkg/core/versioning"
	"github.com/urfave/cli/v2"
)

var ParameterStoreLogCommand = &cli.Command{
	Name:      "log",
	Usage:     "Show parameter version changes",
	ArgsUsage: "<parameter-id> [start-version-id|start-version-label]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "pretty-json",
			Aliases: []string{"j"},
			Value:   false,
			Usage:   "Pretty printing for JSON (keys are automatically sorted)",
		},
		&cli.BoolFlag{
			Name:    "with-decryption",
			Aliases: []string{"S"},
			Value:   false,
			Usage:   "Decrypt parameters (required for SecureString)",
		},
		&cli.IntFlag{
			Name:  "max-results",
			Value: 5,
			Usage: "Number of versions to show",
		},
		&cli.IntFlag{
			Name:  "max-results-to-search",
			Value: 50,
			Usage: "Number of versions to search",
		},
	},
	Action: withParameterStore(func(c *cli.Context) error {
		deps := actions.GetDependencies(c.Context)
		parameterID := c.Args().First()
		if parameterID == "" {
			return fmt.Errorf("parameter-id is required")
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
			Name:               parameterID,
			PrettyJSON:         c.Bool("pretty-json"),
			Version:            version,
			MaxResults:         typeconv.Ref(int32(maxResults)),
			MaxResultsToSearch: typeconv.Ref(int32(maxResultsToSearch)),
		})
	}),
}

func init() {
	ParameterStoreCommand.Subcommands = append(ParameterStoreCommand.Subcommands, ParameterStoreLogCommand)
}
