package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/terminal"
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
	// Mode determines how to handle conflicts with existing agent memory.
	Mode stagingusecase.StashMode
}

// Run executes the stash pop command.
func (r *StashPopRunner) Run(ctx context.Context, opts StashPopOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.StashPopInput{
		Service: opts.Service,
		Keep:    opts.Keep,
		Mode:    opts.Mode,
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
			Usage: "Skip confirmation prompt",
		},
		&cli.BoolFlag{
			Name:  "merge",
			Usage: "Merge with existing agent memory (default)",
		},
		&cli.BoolFlag{
			Name:  "overwrite",
			Usage: "Overwrite agent memory",
		},
		&cli.BoolFlag{
			Name:  "passphrase-stdin",
			Usage: "Read passphrase from stdin (for scripts/automation)",
		},
	}
}

// stashPopMutuallyExclusiveFlags returns the mutually exclusive flags constraint.
func stashPopMutuallyExclusiveFlags() []cli.MutuallyExclusiveFlags {
	return []cli.MutuallyExclusiveFlags{
		{
			Flags: [][]cli.Flag{
				{&cli.BoolFlag{Name: "merge"}},
				{&cli.BoolFlag{Name: "overwrite"}},
			},
		},
	}
}

// StashPopModeInput holds the input for determining stash pop mode.
type StashPopModeInput struct {
	MergeFlag     bool
	OverwriteFlag bool
	HasChanges    bool
	ItemCount     int
	IsTTY         bool
}

// StashPopModeResult holds the result of mode selection.
type StashPopModeResult struct {
	Mode      stagingusecase.StashMode
	Cancelled bool
}

// StashPopModeChooser determines the stash pop mode based on flags and user input.
type StashPopModeChooser struct {
	Prompter *confirm.Prompter
	Stderr   io.Writer
	Stdout   io.Writer
}

// ChooseMode determines the stash pop mode, prompting interactively if needed.
func (c *StashPopModeChooser) ChooseMode(input StashPopModeInput) (StashPopModeResult, error) {
	// Explicit flag takes precedence
	if input.OverwriteFlag {
		return StashPopModeResult{Mode: stagingusecase.StashModeOverwrite}, nil
	}

	if input.MergeFlag {
		return StashPopModeResult{Mode: stagingusecase.StashModeMerge}, nil
	}

	// No explicit flag - check if we need to prompt
	// Only prompt if agent has changes and TTY available
	if input.HasChanges && input.IsTTY {
		output.Warning(c.Stderr, "Agent already has %d staged change(s).", input.ItemCount)

		choice, err := c.Prompter.ConfirmChoice("How do you want to proceed?", []confirm.Choice{
			{Label: "Merge", Description: "combine stashed changes with existing"},
			{Label: "Overwrite", Description: "replace existing with stashed changes"},
			{Label: "Cancel", Description: "abort operation"},
		})
		if err != nil {
			return StashPopModeResult{}, fmt.Errorf("failed to get confirmation: %w", err)
		}

		switch choice {
		case 0: // Merge
			return StashPopModeResult{Mode: stagingusecase.StashModeMerge}, nil
		case 1: // Overwrite
			return StashPopModeResult{Mode: stagingusecase.StashModeOverwrite}, nil
		default: // Cancel or error
			return StashPopModeResult{Cancelled: true}, nil
		}
	}

	// Default to merge when no prompt needed
	return StashPopModeResult{Mode: stagingusecase.StashModeMerge}, nil
}

// stashPopAction creates the action function for stash pop commands.
func stashPopAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		scope := staging.AWSScope(identity.AccountID, identity.Region)

		fileStore, err := fileStoreForReading(cmd, scope, false)
		if err != nil {
			return err
		}

		agentStore := agent.NewStore(scope)

		err = lifecycle.ExecuteWrite0(ctx, agentStore, lifecycle.CmdStashPop, func() error {
			// Check if agent has existing changes
			existingState, err := agentStore.Drain(ctx, service, true) // keep=true to peek
			if err != nil {
				return fmt.Errorf("failed to check agent state: %w", err)
			}

			// Use mode chooser to determine mode
			chooser := &StashPopModeChooser{
				Prompter: &confirm.Prompter{
					Stdin:     cmd.Root().Reader,
					Stdout:    cmd.Root().Writer,
					Stderr:    cmd.Root().ErrWriter,
					AccountID: identity.AccountID,
					Region:    identity.Region,
				},
				Stderr: cmd.Root().ErrWriter,
				Stdout: cmd.Root().Writer,
			}

			result, err := chooser.ChooseMode(StashPopModeInput{
				MergeFlag:     cmd.Bool("merge"),
				OverwriteFlag: cmd.Bool("overwrite"),
				HasChanges:    !existingState.IsEmpty(),
				ItemCount:     existingState.TotalCount(),
				IsTTY:         terminal.IsTerminalWriter(cmd.Root().ErrWriter),
			})
			if err != nil {
				return err
			}

			if result.Cancelled {
				output.Info(cmd.Root().Writer, "Operation cancelled.")

				return nil
			}

			r := &StashPopRunner{
				UseCase: &stagingusecase.StashPopUseCase{
					FileStore:  fileStore,
					AgentStore: agentStore,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, StashPopOptions{
				Service: service,
				Keep:    cmd.Bool("keep"),
				Mode:    result.Mode,
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

EXAMPLES:
   suve stage stash pop                            Restore from file and delete file
   suve stage stash pop --keep                     Restore from file and keep file
   suve stage stash pop --merge                    Merge with existing agent memory
   suve stage stash pop --overwrite                Overwrite agent memory
   echo "secret" | suve stage stash pop --passphrase-stdin   Decrypt with passphrase from stdin`,
		Flags:                  stashPopFlags(),
		MutuallyExclusiveFlags: stashPopMutuallyExclusiveFlags(),
		Action:                 stashPopAction(""), // Empty service = all services
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

EXAMPLES:
   suve stage %s stash pop                            Restore from file
   suve stage %s stash pop --keep                     Restore from file and keep in file
   suve stage %s stash pop --merge                    Merge with existing agent memory
   suve stage %s stash pop --overwrite                Overwrite agent memory
   echo "secret" | suve stage %s stash pop --passphrase-stdin   Decrypt with passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:                  stashPopFlags(),
		MutuallyExclusiveFlags: stashPopMutuallyExclusiveFlags(),
		Action:                 stashPopAction(service),
	}
}
