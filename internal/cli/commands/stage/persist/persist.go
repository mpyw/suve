// Package persist provides the stage persist command.
package persist

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging/agent/client"
	"github.com/mpyw/suve/internal/staging/file"
)

// Command returns the stage persist command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "persist",
		Usage: "Save staged changes from memory to file",
		Description: `Save staged changes from the in-memory agent to a file.

This command saves the current staging state from the agent daemon
to the persistent file storage (~/.suve/{accountID}/{region}/stage.json).

By default, the agent's memory is cleared after persisting.
Use --keep to retain the staged changes in memory.

EXAMPLES:
   suve stage persist                            Save to file and clear agent memory
   suve stage persist --keep                     Save to file and keep agent memory
   echo "secret" | suve stage persist --passphrase-stdin   Use passphrase from stdin`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "keep",
				Usage: "Keep staged changes in agent memory after persisting",
			},
			&cli.BoolFlag{
				Name:  "passphrase-stdin",
				Usage: "Read passphrase from stdin (for scripts/automation)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			agentStore := client.NewStore(identity.AccountID, identity.Region)
			keep := cmd.Bool("keep")

			// Drain state from agent (keep for now, will clear after successful file write if needed)
			state, err := agentStore.Drain(ctx, true)
			if err != nil {
				return fmt.Errorf("failed to get state from agent: %w", err)
			}

			// Check if there's anything to persist
			if state.IsEmpty() {
				return errors.New("no staged changes to persist")
			}

			// Get passphrase
			prompter := &passphrase.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			var pass string
			if cmd.Bool("passphrase-stdin") {
				pass, err = prompter.ReadFromStdin()
				if err != nil {
					return fmt.Errorf("failed to read passphrase from stdin: %w", err)
				}
			} else if terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
				pass, err = prompter.PromptForEncrypt()
				if err != nil {
					if errors.Is(err, passphrase.ErrCancelled) {
						return nil
					}
					return fmt.Errorf("failed to get passphrase: %w", err)
				}
			} else {
				prompter.WarnNonTTY()
				// pass remains empty = plain text
			}

			// Create file store with passphrase for Persist operation
			fileStore, err := file.NewStoreWithPassphrase(identity.AccountID, identity.Region, pass)
			if err != nil {
				return fmt.Errorf("failed to create file store: %w", err)
			}

			if err := fileStore.Persist(ctx, state); err != nil {
				return fmt.Errorf("failed to save state to file: %w", err)
			}

			// Clear agent memory unless --keep is specified
			encrypted := pass != ""
			if !keep {
				// Drain with keep=false to clear memory
				if _, err := agentStore.Drain(ctx, false); err != nil {
					return fmt.Errorf("failed to clear agent memory: %w", err)
				}
				if encrypted {
					_, _ = fmt.Fprintln(cmd.Root().Writer, "Staged changes persisted to file (encrypted) and cleared from memory")
				} else {
					_, _ = fmt.Fprintln(cmd.Root().Writer, "Staged changes persisted to file and cleared from memory")
				}
			} else {
				if encrypted {
					_, _ = fmt.Fprintln(cmd.Root().Writer, "Staged changes persisted to file (encrypted, kept in memory)")
				} else {
					_, _ = fmt.Fprintln(cmd.Root().Writer, "Staged changes persisted to file (kept in memory)")
				}
			}

			// Display warning about plain-text storage only if not encrypted
			if !encrypted {
				_, _ = fmt.Fprintf(cmd.Root().ErrWriter, "%s Note: secrets are stored as plain text.\n", colors.Warning("!"))
			}

			return nil
		},
	}
}
