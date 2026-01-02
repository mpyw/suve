// Package commands provides the command-line interface for suve.
package commands

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/param"
	"github.com/mpyw/suve/internal/cli/commands/secret"
	"github.com/mpyw/suve/internal/cli/commands/stage"
)

// MakeApp creates a new CLI application instance.
func MakeApp() *cli.Command {
	return &cli.Command{
		Name:    "suve",
		Usage:   "Git-like CLI for AWS Parameter Store and Secrets Manager",
		Version: "0.1.0",
		Commands: []*cli.Command{
			param.Command(),
			secret.Command(),
			stage.Command(),
		},
		CommandNotFound: func(_ context.Context, cmd *cli.Command, command string) {
			_ = cli.ShowAppHelp(cmd)
			w := lo.CoalesceOrEmpty(cmd.Root().ErrWriter, cmd.Root().Writer)
			_, _ = fmt.Fprintf(w, "\nCommand not found: %s\n", command)
		},
	}
}

// App is the main CLI application.
var App = MakeApp()
