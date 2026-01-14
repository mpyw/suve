// Package commands provides the command-line interface for suve.
package commands

import (
	"context"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/param"
	"github.com/mpyw/suve/internal/cli/commands/secret"
	"github.com/mpyw/suve/internal/cli/commands/stage"
	"github.com/mpyw/suve/internal/cli/output"
)

// Version is set by goreleaser via ldflags.
//
//nolint:gochecknoglobals // build-time variable set by ldflags
var Version = "dev"

// MakeApp creates a new CLI application instance.
func MakeApp() *cli.Command {
	return &cli.Command{
		Name:    "suve",
		Usage:   "Git-like CLI for AWS Parameter Store and Secrets Manager",
		Version: Version,
		Commands: []*cli.Command{
			param.Command(),
			secret.Command(),
			stage.Command(),
		},
		CommandNotFound: func(_ context.Context, cmd *cli.Command, command string) {
			_ = cli.ShowAppHelp(cmd)
			w := lo.CoalesceOrEmpty(cmd.Root().ErrWriter, cmd.Root().Writer)
			output.Println(w, "")
			output.Warning(w, "Command not found: %s", command)
		},
	}
}

// App is the main CLI application.
//
//nolint:gochecknoglobals // singleton CLI app instance
var App = MakeApp()
