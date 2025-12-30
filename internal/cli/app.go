// Package cli provides the command-line interface for suve.
package cli

import (
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/cli/sm"
	"github.com/mpyw/suve/internal/cli/ssm"
)

// Version is set via ldflags at build time.
var Version = "dev"

// App is the main CLI application.
var App = &cli.App{
	Name:    "suve",
	Usage:   "Git-like CLI for AWS Parameter Store and Secrets Manager",
	Version: Version,
	Commands: []*cli.Command{
		ssm.Command(),
		sm.Command(),
	},
}
