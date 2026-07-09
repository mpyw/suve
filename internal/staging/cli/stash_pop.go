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
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
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
	// Mode determines how to handle conflicts with the existing working staging area.
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
			Name:  flagYes,
			Usage: usageSkipConfirm,
		},
		&cli.BoolFlag{
			Name:  flagPassphraseStdin,
			Usage: usagePassphraseStdin,
		},
	}
}

// stashPopMutuallyExclusiveFlags returns the mutually exclusive --merge /
// --overwrite constraint. The flags are declared ONLY here (see the push
// equivalent): a duplicate copy in stashPopFlags would shadow these under
// urfave/cli v3 and silently disable the exclusivity check.
func stashPopMutuallyExclusiveFlags() []cli.MutuallyExclusiveFlags {
	return []cli.MutuallyExclusiveFlags{
		{
			Flags: [][]cli.Flag{
				{&cli.BoolFlag{Name: flagMerge, Usage: "Merge with the existing working staging area (default)"}},
				{&cli.BoolFlag{Name: flagOverwrite, Usage: "Overwrite the working staging area"}},
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
	// Only prompt if the working area has changes and TTY available
	if input.HasChanges && input.IsTTY {
		output.Warning(c.Stderr, "Working staging area already has %d staged change(s).", input.ItemCount)

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
func stashPopAction(service staging.Service, resolver staging.ScopeResolver) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		resolved, err := resolveScope(ctx, resolver)
		if err != nil {
			return err
		}

		scope := resolved.Scope

		stashStore, err := fileStoreForReading(cmd, scope, false)
		if err != nil {
			return err
		}

		working, err := file.NewWorkingStore(scope)
		if err != nil {
			return fmt.Errorf("failed to create staging store: %w", err)
		}

		// Check if the working staging area has existing changes
		existingState, err := working.Drain(ctx, service, true) // keep=true to peek
		if err != nil {
			return fmt.Errorf("failed to check working staging area: %w", err)
		}

		// Use mode chooser to determine mode
		chooser := &StashPopModeChooser{
			Prompter: &confirm.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
				Target: resolved.Target,
			},
			Stderr: cmd.Root().ErrWriter,
			Stdout: cmd.Root().Writer,
		}

		result, err := chooser.ChooseMode(StashPopModeInput{
			MergeFlag:     cmd.Bool(flagMerge),
			OverwriteFlag: cmd.Bool(flagOverwrite),
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
				Stash:   stashStore,
				Working: working,
			},
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		return r.Run(ctx, StashPopOptions{
			Service: service,
			Keep:    cmd.Bool("keep"),
			Mode:    result.Mode,
		})
	}
}

// newGlobalStashPopCommand creates a global stash pop command that operates on all services.
func newGlobalStashPopCommand(resolver staging.ScopeResolver) *cli.Command {
	return &cli.Command{
		Name:  "pop",
		Usage: "Restore staged changes from the stash file into the working staging area",
		Description: `Restore staged changes from the stash file into the working staging area.

This command loads the staging state from the stash file
(~/.suve/staging/aws/{accountID}/{region}/stash.json) into the working staging area
(~/.suve/staging/aws/{accountID}/{region}/{param,secret}.json).

By default, the stash file is deleted after restoring.
Use --keep to retain the stash file after popping (like 'git stash apply').

EXAMPLES:
   suve stage stash pop                            Restore from stash and delete the stash file
   suve stage stash pop --keep                     Restore from stash and keep the stash file
   suve stage stash pop --merge                    Merge with the existing working staging area
   suve stage stash pop --overwrite                Overwrite the working staging area
   echo "secret" | suve stage stash pop --passphrase-stdin   Decrypt with passphrase from stdin`,
		Flags:                  stashPopFlags(),
		MutuallyExclusiveFlags: stashPopMutuallyExclusiveFlags(),
		Action:                 stashPopAction("", resolver), // Empty service = all services
	}
}

// newStashPopCommand creates a service-specific stash pop command with the given config.
func newStashPopCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "pop",
		Usage: fmt.Sprintf("Restore staged %s changes from the stash file into the working staging area", cfg.ItemName),
		Description: fmt.Sprintf(`Restore staged %s changes from the stash file into the working staging area.

This command loads the staging state for %ss from the stash file
(~/.suve/staging/aws/{accountID}/{region}/stash.json) into the working staging area
(~/.suve/staging/aws/{accountID}/{region}/{param,secret}.json).

By default, the %s entries are removed from the stash file after restoring.
Use --keep to retain them in the stash file.

EXAMPLES:
   suve stage %s stash pop                            Restore from stash
   suve stage %s stash pop --keep                     Restore from stash and keep in the stash file
   suve stage %s stash pop --merge                    Merge with the existing working staging area
   suve stage %s stash pop --overwrite                Overwrite the working staging area
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
		Action:                 stashPopAction(service, cfg.ScopeResolver),
	}
}
