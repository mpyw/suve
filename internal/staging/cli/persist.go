// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/file"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// PersistRunner executes persist operations using a usecase.
type PersistRunner struct {
	UseCase   *stagingusecase.PersistUseCase
	Stdout    io.Writer
	Stderr    io.Writer
	Encrypted bool // Whether the file is encrypted (for output messages)
}

// PersistOptions holds options for the persist command.
type PersistOptions struct {
	// Service filters the persist to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the agent memory after persisting.
	Keep bool
}

// Run executes the persist command.
func (r *PersistRunner) Run(ctx context.Context, opts PersistOptions) error {
	_, err := r.UseCase.Execute(ctx, stagingusecase.PersistInput{
		Service: opts.Service,
		Keep:    opts.Keep,
	})

	if err != nil {
		// Check for non-fatal error (state was written but agent cleanup failed)
		var persistErr *stagingusecase.PersistError
		if errors.As(err, &persistErr) && persistErr.NonFatal {
			output.Warn(r.Stderr, "Warning: %v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if opts.Keep {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes persisted to file (encrypted, kept in memory)")
		} else {
			output.Success(r.Stdout, "Staged changes persisted to file (kept in memory)")
		}
	} else {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes persisted to file (encrypted) and cleared from memory")
		} else {
			output.Success(r.Stdout, "Staged changes persisted to file and cleared from memory")
		}
	}

	// Display warning about plain-text storage only if not encrypted
	if !r.Encrypted {
		output.Warn(r.Stderr, "Note: secrets are stored as plain text.")
	}

	return nil
}

// NewPersistCommand creates a service-specific persist command with the given config.
func NewPersistCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "persist",
		Usage: fmt.Sprintf("Save staged %s changes from memory to file", cfg.ItemName),
		Description: fmt.Sprintf(`Save staged %s changes from the in-memory agent to a file.

This command saves the staging state for %ss from the agent daemon
to the persistent file storage (~/.suve/{accountID}/{region}/stage.json).

By default, the %s entries are cleared from agent memory after persisting.
Use --keep to retain them in memory.

EXAMPLES:
   suve stage %s persist                            Save to file and clear agent memory
   suve stage %s persist --keep                     Save to file and keep agent memory
   echo "secret" | suve stage %s persist --passphrase-stdin   Use passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
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

			agentStore := agent.NewStore(identity.AccountID, identity.Region)

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

			fileStore, err := file.NewStoreWithPassphrase(identity.AccountID, identity.Region, pass)
			if err != nil {
				return fmt.Errorf("failed to create file store: %w", err)
			}

			r := &PersistRunner{
				UseCase: &stagingusecase.PersistUseCase{
					AgentStore: agentStore,
					FileStore:  fileStore,
				},
				Stdout:    cmd.Root().Writer,
				Stderr:    cmd.Root().ErrWriter,
				Encrypted: pass != "",
			}
			return r.Run(ctx, PersistOptions{
				Service: service,
				Keep:    cmd.Bool("keep"),
			})
		},
	}
}
