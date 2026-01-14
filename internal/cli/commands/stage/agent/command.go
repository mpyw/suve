// Package agent provides the stage agent subcommand.
package agent

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	agentcfg "github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon"
)

// Command returns the stage agent command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "agent",
		Usage: "Manage the staging agent daemon",
		Description: `Manage the in-memory staging agent daemon.

The agent daemon runs in the background and stores staged changes in secure memory.
It automatically starts when needed and stops when staging becomes empty.

COMMANDS:
   start    Start the agent daemon (usually done automatically)
   stop     Stop the agent daemon

EXAMPLES:
   suve stage agent start    Start the agent daemon manually
   suve stage agent stop     Stop the agent daemon`,
		Commands: []*cli.Command{
			startCommand(),
			stopCommand(),
		},
	}
}

func startCommand() *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "Start the staging agent daemon",
		Description: `Start the staging agent daemon in the background.

The daemon stores staged changes in secure memory (mlock'd, encrypted).
This command is usually called automatically when staging operations are performed.

The daemon will automatically shut down when all staged changes are cleared.

Set ` + agentcfg.EnvDaemonManualMode + `=1 to enable manual mode (disables auto-start and auto-shutdown).`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "account",
				Usage: "AWS account ID (required, usually passed automatically)",
			},
			&cli.StringFlag{
				Name:  "region",
				Usage: "AWS region (required, usually passed automatically)",
			},
			&cli.BoolFlag{
				Name:   "foreground",
				Usage:  "Run daemon in foreground (used internally by spawner)",
				Hidden: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			accountID := cmd.String("account")
			region := cmd.String("region")
			foreground := cmd.Bool("foreground")

			// If not passed via flags, get from AWS identity
			if accountID == "" || region == "" {
				identity, err := infra.GetAWSIdentity(ctx)
				if err != nil {
					return fmt.Errorf("failed to get AWS identity: %w", err)
				}

				accountID = identity.AccountID
				region = identity.Region
			}

			// Foreground mode: run daemon directly (used by spawner)
			if foreground {
				runner := daemon.NewRunner(accountID, region, agentcfg.DaemonOptions()...)

				return runner.Run(ctx)
			}

			// Background mode: spawn daemon via launcher
			launcher := daemon.NewLauncher(accountID, region)

			// Check if already running
			if err := launcher.Ping(ctx); err == nil {
				output.Info(cmd.Root().Writer, "Agent is already running.")

				return nil
			}

			// Start daemon in background
			if err := launcher.EnsureRunning(ctx); err != nil {
				return err
			}

			output.Info(cmd.Root().Writer, "Agent started.")

			return nil
		},
	}
}

func stopCommand() *cli.Command {
	return &cli.Command{
		Name:  "stop",
		Usage: "Stop the staging agent daemon",
		Description: `Stop the staging agent daemon.

This command sends a shutdown signal to the running daemon.
Note: Any staged changes in memory will be lost unless persisted first.`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			launcher := daemon.NewLauncher(identity.AccountID, identity.Region)
			if err := launcher.Shutdown(ctx); err != nil {
				output.Warning(cmd.Root().ErrWriter, "%v", err)

				return nil
			}

			output.Info(cmd.Root().Writer, "Agent stopped.")

			return nil
		},
	}
}
