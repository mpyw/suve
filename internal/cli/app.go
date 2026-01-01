// Package cli provides the command-line interface for suve.
package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/push"
	"github.com/mpyw/suve/internal/cli/reset"
	"github.com/mpyw/suve/internal/cli/sm"
	"github.com/mpyw/suve/internal/cli/ssm"
	"github.com/mpyw/suve/internal/cli/status"
)

// MakeApp creates a new CLI application instance.
func MakeApp() *cli.Command {
	return &cli.Command{
		Name:    "suve",
		Usage:   "Git-like CLI for AWS Parameter Store and Secrets Manager",
		Version: "0.1.0",
		Commands: []*cli.Command{
			ssm.Command(),
			sm.Command(),
			status.Command(),
			push.Command(),
			reset.Command(),
		},
		CommandNotFound: commandNotFound,
	}
}

func commandNotFound(_ context.Context, cmd *cli.Command, command string) {
	_ = cli.ShowAppHelp(cmd)
	w := cmd.Root().ErrWriter
	if w == nil {
		w = cmd.Root().Writer
	}
	_, _ = fmt.Fprintf(w, "\nCommand not found: %s\n", command)
}

// App is the main CLI application.
var App = MakeApp()
