package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
	usestaging "github.com/mpyw/suve/internal/usecase/staging"
)

// StashPushRunner executes stash push operations using a usecase.
type StashPushRunner struct {
	UseCase   *usestaging.StashPushUseCase
	Stdout    io.Writer
	Stderr    io.Writer
	Encrypted bool // Whether the file is encrypted (for output messages)
}

// StashPushOptions holds options for the stash push command.
type StashPushOptions struct {
	// Service filters the operation to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the working staging area after pushing to the stash file.
	Keep bool
	// Mode determines how to handle existing stash file.
	Mode usestaging.StashMode
}

// Run executes the stash push command.
func (r *StashPushRunner) Run(ctx context.Context, opts StashPushOptions) error {
	_, err := r.UseCase.Execute(ctx, usestaging.StashPushInput{
		Service: opts.Service,
		Keep:    opts.Keep,
		Mode:    opts.Mode,
	})
	if err != nil {
		// Check for non-fatal error (state was written but working-area cleanup failed)
		var persistErr *usestaging.StashPushError
		if errors.As(err, &persistErr) && persistErr.NonFatal {
			output.Warning(r.Stderr, "%v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if opts.Keep {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes stashed to file (encrypted, kept in the working staging area)")
		} else {
			output.Success(r.Stdout, "Staged changes stashed to file (kept in the working staging area)")
		}
	} else {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes stashed to file (encrypted) and cleared from the working staging area")
		} else {
			output.Success(r.Stdout, "Staged changes stashed to file and cleared from the working staging area")
		}
	}

	// Display warning about plain-text storage only if not encrypted
	if !r.Encrypted {
		output.Warn(r.Stderr, "Secrets are stored as plain text.")
	}

	return nil
}

// stashPushFlags returns the common flags for stash push commands.
func stashPushFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "keep",
			Usage: "Keep staged changes in the working staging area after stashing",
		},
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt",
		},
		&cli.BoolFlag{
			Name:  "merge",
			Usage: "Merge with existing stash (default)",
		},
		&cli.BoolFlag{
			Name:  "overwrite",
			Usage: "Overwrite existing stash",
		},
		&cli.BoolFlag{
			Name:  "passphrase-stdin",
			Usage: "Read passphrase from stdin (for scripts/automation)",
		},
	}
}

// stashPushMutuallyExclusiveFlags returns the mutually exclusive flags constraint.
func stashPushMutuallyExclusiveFlags() []cli.MutuallyExclusiveFlags {
	return []cli.MutuallyExclusiveFlags{
		{
			Flags: [][]cli.Flag{
				{&cli.BoolFlag{Name: "merge"}},
				{&cli.BoolFlag{Name: "overwrite"}},
			},
		},
	}
}

// stashPushAction creates the action function for stash push commands.
func stashPushAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		// Working staging area is the source of the push.
		working, err := file.NewWorkingStore(provider.AWSScope(identity.AccountID, identity.Region))
		if err != nil {
			return fmt.Errorf("failed to create staging store: %w", err)
		}

		// Stash file is the destination of the push.
		basicStashStore, err := file.NewStashStore(provider.AWSScope(identity.AccountID, identity.Region))
		if err != nil {
			return fmt.Errorf("failed to create stash store: %w", err)
		}

		// Determine mode based on flags
		mergeFlag := cmd.Bool("merge")
		overwriteFlag := cmd.Bool("overwrite")
		skipConfirm := cmd.Bool("yes")

		var mode usestaging.StashMode

		switch {
		case overwriteFlag:
			mode = usestaging.StashModeOverwrite
		case mergeFlag || skipConfirm:
			// --merge or --yes (without explicit mode) defaults to merge
			mode = usestaging.StashModeMerge
		default:
			// No flag specified - check if we need to prompt
			exists, err := basicStashStore.Exists()
			if err != nil {
				return fmt.Errorf("failed to check stash file: %w", err)
			}

			// Only prompt for global push when file exists and TTY available
			// Service-specific push defaults to merge (preserves other services)
			if exists && service == "" && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
				// Count existing items for the prompt message
				existingState, err := basicStashStore.Drain(ctx, "", true)
				if err != nil {
					return fmt.Errorf("failed to read stash file: %w", err)
				}

				itemCount := existingState.TotalCount()

				confirmPrompter := &confirm.Prompter{
					Stdin:  cmd.Root().Reader,
					Stdout: cmd.Root().Writer,
					Stderr: cmd.Root().ErrWriter,
				}

				output.Warning(cmd.Root().ErrWriter, "Stash file already exists with %d item(s).", itemCount)

				choice, err := confirmPrompter.ConfirmChoice("How do you want to proceed?", []confirm.Choice{
					{Label: "Merge", Description: "combine with existing stash"},
					{Label: "Overwrite", Description: "replace existing stash"},
					{Label: "Cancel", Description: "abort operation"},
				})
				if err != nil {
					return fmt.Errorf("failed to get confirmation: %w", err)
				}

				switch choice {
				case 0: // Merge
					mode = usestaging.StashModeMerge
				case 1: // Overwrite
					mode = usestaging.StashModeOverwrite
				default: // Cancel or error
					output.Info(cmd.Root().Writer, "Operation cancelled.")

					return nil
				}
			} else {
				// Default to merge when no TTY or service-specific
				mode = usestaging.StashModeMerge
			}
		}

		// Get passphrase
		prompter := &passphrase.Prompter{
			Stdin:  cmd.Root().Reader,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		var pass string

		switch {
		case cmd.Bool("passphrase-stdin"):
			pass, err = prompter.ReadFromStdin()
			if err != nil {
				return fmt.Errorf("failed to read passphrase from stdin: %w", err)
			}
		case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
			pass, err = prompter.PromptForEncrypt()
			if err != nil {
				if errors.Is(err, passphrase.ErrCancelled) {
					return nil
				}

				return fmt.Errorf("failed to get passphrase: %w", err)
			}
		default:
			prompter.WarnNonTTY()
			// pass remains empty = plain text
		}

		stashStore, err := file.NewStashStoreWithPassphrase(provider.AWSScope(identity.AccountID, identity.Region), pass)
		if err != nil {
			return fmt.Errorf("failed to create stash store: %w", err)
		}

		r := &StashPushRunner{
			UseCase: &usestaging.StashPushUseCase{
				Working: working,
				Stash:   stashStore,
			},
			Stdout:    cmd.Root().Writer,
			Stderr:    cmd.Root().ErrWriter,
			Encrypted: pass != "",
		}

		err = r.Run(ctx, StashPushOptions{
			Service: service,
			Keep:    cmd.Bool("keep"),
			Mode:    mode,
		})
		if err != nil {
			// Handle "nothing to stash" gracefully (working area empty)
			if errors.Is(err, usestaging.ErrNothingToStashPush) {
				output.Info(cmd.Root().Writer, "No staged changes to persist.")

				return nil
			}

			return err
		}

		return nil
	}
}

// newGlobalStashPushCommand creates a global stash push command that operates on all services.
func newGlobalStashPushCommand() *cli.Command {
	return &cli.Command{
		Name:  "push",
		Usage: "Save staged changes from the working staging area to the stash file",
		Description: `Save staged changes from the working staging area to the stash file.

This command saves the current staging state from the working staging area
(~/.suve/staging/aws/{accountID}/{region}/{param,secret}.json) to the stash file
(~/.suve/staging/aws/{accountID}/{region}/stash.json).

By default, the working staging area is cleared after stashing.
Use --keep to retain the staged changes in the working staging area.

EXAMPLES:
   suve stage stash push                            Save to stash and clear the working staging area
   suve stage stash push --keep                     Save to stash and keep the working staging area
   echo "secret" | suve stage stash push --passphrase-stdin   Use passphrase from stdin`,
		Flags:                  stashPushFlags(),
		MutuallyExclusiveFlags: stashPushMutuallyExclusiveFlags(),
		Action:                 stashPushAction(""), // Empty service = all services
	}
}

// newStashPushCommand creates a service-specific stash push command with the given config.
func newStashPushCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "push",
		Usage: fmt.Sprintf("Save staged %s changes from the working staging area to the stash file", cfg.ItemName),
		Description: fmt.Sprintf(`Save staged %s changes from the working staging area to the stash file.

This command saves the staging state for %ss from the working staging area
(~/.suve/staging/aws/{accountID}/{region}/{param,secret}.json) to the stash file
(~/.suve/staging/aws/{accountID}/{region}/stash.json).

By default, the %s entries are cleared from the working staging area after stashing.
Use --keep to retain them in the working staging area.

EXAMPLES:
   suve stage %s stash push                            Save to stash and clear the working staging area
   suve stage %s stash push --keep                     Save to stash and keep the working staging area
   echo "secret" | suve stage %s stash push --passphrase-stdin   Use passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:                  stashPushFlags(),
		MutuallyExclusiveFlags: stashPushMutuallyExclusiveFlags(),
		Action:                 stashPushAction(service),
	}
}
