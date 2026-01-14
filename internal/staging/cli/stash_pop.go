package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StashPopRunner executes stash pop operations using a usecase.
type StashPopRunner struct {
	UseCase *stagingusecase.StashPopUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// StashPopOptions holds options for the stash pop command.
type StashPopOptions struct {
	// Service filters the operation to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the file after popping.
	Keep bool
	// Overwrite overwrites agent memory without checking for conflicts.
	Overwrite bool
	// Merge combines file changes with existing agent memory.
	Merge bool
}

// Run executes the stash pop command.
func (r *StashPopRunner) Run(ctx context.Context, opts StashPopOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.StashPopInput{
		Service:   opts.Service,
		Keep:      opts.Keep,
		Overwrite: opts.Overwrite,
		Merge:     opts.Merge,
	})
	if err != nil {
		// Check for non-fatal error (state was written but file cleanup failed)
		var drainErr *stagingusecase.StashPopError
		if errors.As(err, &drainErr) && drainErr.NonFatal {
			output.Warning(r.Stderr, "%v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if result.Merged {
		if opts.Keep {
			output.Success(r.Stdout, "Stashed changes restored and merged (file kept)")
		} else {
			output.Success(r.Stdout, "Stashed changes restored and merged (file deleted)")
		}
	} else {
		if opts.Keep {
			output.Success(r.Stdout, "Stashed changes restored (file kept)")
		} else {
			output.Success(r.Stdout, "Stashed changes restored and file deleted")
		}
	}

	return nil
}

// stashPopFlags returns the common flags for stash pop commands.
func stashPopFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "keep",
			Usage: "Keep the file after restoring into memory",
		},
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt (uses merge mode by default)",
		},
		&cli.BoolFlag{
			Name:  "merge",
			Usage: "Merge with existing agent memory (same as --yes, explicit mode)",
		},
		&cli.BoolFlag{
			Name:  "overwrite",
			Usage: "Overwrite agent memory instead of merging",
		},
		&cli.BoolFlag{
			Name:  "passphrase-stdin",
			Usage: "Read passphrase from stdin (for scripts/automation)",
		},
	}
}

// stashPopAction creates the action function for stash pop commands.
func stashPopAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		fileStore, err := fileStoreForReading(cmd, identity.AccountID, identity.Region, false)
		if err != nil {
			return err
		}

		agentStore := agent.NewStore(identity.AccountID, identity.Region)

		err = lifecycle.ExecuteWrite0(ctx, agentStore, lifecycle.CmdStashPop, func() error {
			r := &StashPopRunner{
				UseCase: &stagingusecase.StashPopUseCase{
					FileStore:  fileStore,
					AgentStore: agentStore,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, StashPopOptions{
				Service:   service,
				Keep:      cmd.Bool("keep"),
				Overwrite: cmd.Bool("overwrite"),
				Merge:     cmd.Bool("merge") || cmd.Bool("yes"),
			})
		})

		return err
	}
}

// newGlobalStashPopCommand creates a global stash pop command that operates on all services.
func newGlobalStashPopCommand() *cli.Command {
	return &cli.Command{
		Name:  "pop",
		Usage: "Restore staged changes from file into memory",
		Description: `Restore staged changes from file into the in-memory agent.

This command loads the staging state from the persistent file storage
(~/.suve/{accountID}/{region}/stage.json) into the agent daemon.

By default, the file is deleted after restoring.
Use --keep to retain the file after popping (same as 'stash apply').

If the agent already has staged changes, you'll be prompted to confirm
the action. Use --yes to skip the prompt and merge, or --overwrite
to replace the existing memory changes.

EXAMPLES:
   suve stage stash pop                            Restore from file and delete file
   suve stage stash pop --keep                     Restore from file and keep file
   suve stage stash pop --yes                      Merge with agent memory without prompt
   suve stage stash pop --overwrite                Overwrite agent memory
   echo "secret" | suve stage stash pop --passphrase-stdin   Decrypt with passphrase from stdin`,
		Flags:  stashPopFlags(),
		Action: stashPopAction(""), // Empty service = all services
	}
}

// newStashPopCommand creates a service-specific stash pop command with the given config.
func newStashPopCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "pop",
		Usage: fmt.Sprintf("Restore staged %s changes from file into memory", cfg.ItemName),
		Description: fmt.Sprintf(`Restore staged %s changes from file into the in-memory agent.

This command loads the staging state for %ss from the persistent file storage
(~/.suve/{accountID}/{region}/stage.json) into the agent daemon.

By default, the %s entries are removed from the file after restoring.
Use --keep to retain them in the file.

If the agent already has staged %s changes, you'll be prompted to confirm
the action. Use --yes to skip the prompt and merge, or --overwrite
to replace the existing memory changes.

EXAMPLES:
   suve stage %s stash pop                            Restore from file
   suve stage %s stash pop --keep                     Restore from file and keep in file
   suve stage %s stash pop --yes                      Merge with agent memory without prompt
   suve stage %s stash pop --overwrite                Overwrite agent memory
   echo "secret" | suve stage %s stash pop --passphrase-stdin   Decrypt with passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:  stashPopFlags(),
		Action: stashPopAction(service),
	}
}
