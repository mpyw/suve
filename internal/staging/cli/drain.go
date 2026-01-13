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

// DrainRunner executes drain operations using a usecase.
type DrainRunner struct {
	UseCase *stagingusecase.DrainUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// DrainOptions holds options for the drain command.
type DrainOptions struct {
	// Service filters the drain to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the file after draining.
	Keep bool
	// Force overwrites agent memory without checking for conflicts.
	Force bool
	// Merge combines file changes with existing agent memory.
	Merge bool
}

// Run executes the drain command.
func (r *DrainRunner) Run(ctx context.Context, opts DrainOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.DrainInput{
		Service: opts.Service,
		Keep:    opts.Keep,
		Force:   opts.Force,
		Merge:   opts.Merge,
	})

	if err != nil {
		// Check for non-fatal error (state was written but file cleanup failed)
		var drainErr *stagingusecase.DrainError
		if errors.As(err, &drainErr) && drainErr.NonFatal {
			output.Warn(r.Stderr, "Warning: %v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if result.Merged {
		if opts.Keep {
			output.Success(r.Stdout, "Staged changes loaded and merged from file (file kept)")
		} else {
			output.Success(r.Stdout, "Staged changes loaded and merged from file (file deleted)")
		}
	} else {
		if opts.Keep {
			output.Success(r.Stdout, "Staged changes loaded from file (file kept)")
		} else {
			output.Success(r.Stdout, "Staged changes loaded from file and file deleted")
		}
	}

	return nil
}

// drainFlags returns the common flags for drain commands.
func drainFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "keep",
			Usage: "Keep the file/entries after loading into memory",
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
	}
}

// drainAction creates the action function for drain commands.
func drainAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		// Check if file is encrypted
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

		fileStore, err := file.NewStoreWithPassphrase(identity.AccountID, identity.Region, pass)
		if err != nil {
			return fmt.Errorf("failed to create file store: %w", err)
		}

		agentStore := agent.NewStore(identity.AccountID, identity.Region)

		r := &DrainRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, DrainOptions{
			Service: service,
			Keep:    cmd.Bool("keep"),
			Force:   cmd.Bool("force"),
			Merge:   cmd.Bool("merge"),
		})
	}
}

// NewGlobalDrainCommand creates a global drain command that operates on all services.
func NewGlobalDrainCommand() *cli.Command {
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
		Flags:  drainFlags(),
		Action: drainAction(""), // Empty service = all services
	}
}

// NewDrainCommand creates a service-specific drain command with the given config.
func NewDrainCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "drain",
		Usage: fmt.Sprintf("Load staged %s changes from file into memory", cfg.ItemName),
		Description: fmt.Sprintf(`Load staged %s changes from file into the in-memory agent.

This command loads the staging state for %ss from the persistent file storage
(~/.suve/{accountID}/{region}/stage.json) into the agent daemon.

By default, the %s entries are removed from the file after loading.
Use --keep to retain them in the file.

If the agent already has staged %s changes, you'll be prompted to confirm
the action. Use --force to skip the prompt and overwrite, or --merge
to merge the file changes with existing memory changes.

EXAMPLES:
   suve stage %s drain                            Load from file
   suve stage %s drain --keep                     Load from file and keep in file
   suve stage %s drain --force                    Overwrite agent memory without prompt
   suve stage %s drain --merge                    Merge file with existing memory
   echo "secret" | suve stage %s drain --passphrase-stdin   Decrypt with passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:  drainFlags(),
		Action: drainAction(service),
	}
}
