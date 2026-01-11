// Package agent provides the stage agent subcommand.
package agent

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/staging/agent"
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

The daemon will automatically shut down when all staged changes are cleared.`,
		Action: func(_ context.Context, _ *cli.Command) error {
			daemon := agent.NewDaemon()
			return daemon.Run()
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
			client := agent.NewClient()
			if err := client.Shutdown(ctx); err != nil {
				_, _ = fmt.Fprintf(cmd.Root().ErrWriter, "Warning: %v\n", err)
				return nil
			}
			_, _ = fmt.Fprintln(cmd.Root().Writer, "Agent stopped")
			return nil
		},
	}
}
