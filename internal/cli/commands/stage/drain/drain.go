// Package drain provides the stage drain command.
package drain

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/file"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// Command returns the stage drain command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "drain",
		Usage: "Load staged changes from file into memory",
		Description: `Load staged changes from file into the in-memory agent.

This command loads the staging state from the persistent file storage
(~/.suve/{accountID}/{region}/stage.json) into the agent daemon.

By default, the file is deleted after loading.
Use --keep to retain the file after draining.

If the agent already has staged changes, you'll be prompted to confirm
the action. Use --force to skip the prompt and overwrite, or --merge
to merge the file changes with existing memory changes.

EXAMPLES:
   suve stage drain                            Load from file and delete file
   suve stage drain --keep                     Load from file and keep file
   suve stage drain --force                    Overwrite agent memory without prompt
   suve stage drain --merge                    Merge file with existing memory
   echo "secret" | suve stage drain --passphrase-stdin   Decrypt with passphrase from stdin`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "keep",
				Usage: "Keep the file after loading into memory",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite agent memory without prompt",
			},
			&cli.BoolFlag{
				Name:  "merge",
				Usage: "Merge file changes with existing memory",
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

			// Check if file is encrypted (need a basic store to check)
			basicFileStore, err := file.NewStore(identity.AccountID, identity.Region)
			if err != nil {
				return fmt.Errorf("failed to create file store: %w", err)
			}

			isEnc, err := basicFileStore.IsEncrypted()
			if err != nil {
				return fmt.Errorf("failed to check file encryption: %w", err)
			}

			var pass string
			if isEnc {
				prompter := &passphrase.Prompter{
					Stdin:  cmd.Root().Reader,
					Stdout: cmd.Root().Writer,
					Stderr: cmd.Root().ErrWriter,
				}
				if cmd.Bool("passphrase-stdin") {
					pass, err = prompter.ReadFromStdin()
					if err != nil {
						return fmt.Errorf("failed to read passphrase from stdin: %w", err)
					}
				} else if terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
					pass, err = prompter.PromptForDecrypt()
					if err != nil {
						return fmt.Errorf("failed to get passphrase: %w", err)
					}
				} else {
					return errors.New("encrypted file cannot be decrypted in non-TTY environment; use --passphrase-stdin")
				}
			}

			// Create file store with passphrase for Drain operation
			fileStore, err := file.NewStoreWithPassphrase(identity.AccountID, identity.Region, pass)
			if err != nil {
				return fmt.Errorf("failed to create file store: %w", err)
			}

			agentStore := agent.NewStore(identity.AccountID, identity.Region)

			r := &stgcli.DrainRunner{
				UseCase: &stagingusecase.DrainUseCase{
					FileStore:  fileStore,
					AgentStore: agentStore,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}
			return r.Run(ctx, stgcli.DrainOptions{
				Keep:  cmd.Bool("keep"),
				Force: cmd.Bool("force"),
				Merge: cmd.Bool("merge"),
			})
		},
	}
}
