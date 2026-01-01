// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/confirm"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/staging"
)

// CommandConfig holds service-specific configuration for building stage commands.
type CommandConfig struct {
	// ServiceName is the service prefix for commands (e.g., "ssm", "sm").
	ServiceName string

	// ItemName is the item name for messages (e.g., "parameter", "secret").
	ItemName string

	// Factory creates a FullStrategy with an initialized AWS client.
	Factory staging.StrategyFactory

	// ParserFactory creates a Parser without AWS client (for status, parsing).
	ParserFactory staging.ParserFactory
}

// NewStatusCommand creates a status command with the given config.
func NewStatusCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     fmt.Sprintf("Show staged %s changes", cfg.ItemName),
		ArgsUsage: "[name]",
		Description: fmt.Sprintf(`Display staged changes for %s.

Without arguments, shows all staged %s changes.
With a %s name, shows the staged change for that specific %s.

Use -v/--verbose to show detailed information including the staged value.

EXAMPLES:
   suve %s stage status              Show all staged %s changes
   suve %s stage status <name>       Show staged change for specific %s
   suve %s stage status -v           Show detailed information`,
			cfg.ServiceName, cfg.ItemName, cfg.ItemName, cfg.ItemName,
			cfg.ServiceName, cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			r := &StatusRunner{
				Strategy: cfg.ParserFactory(),
				Store:    store,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			opts := StatusOptions{
				Verbose: cmd.Bool("verbose"),
			}
			if cmd.Args().Len() > 0 {
				opts.Name = cmd.Args().First()
			}

			return r.Run(ctx, opts)
		},
	}
}

// NewDiffCommand creates a diff command with the given config.
func NewDiffCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between staged and AWS values",
		ArgsUsage: "[name]",
		Description: fmt.Sprintf(`Compare staged values against AWS current values.

If a %s name is specified, shows diff for that %s only.
Otherwise, shows diff for all staged %s %ss.

EXAMPLES:
   suve %s stage diff              Show diff for all staged %s %ss
   suve %s stage diff <name>       Show diff for specific %s
   suve %s stage diff -j           Show diff with JSON formatting`,
			cfg.ItemName, cfg.ItemName, cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName, cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			var name string
			if cmd.Args().Len() > 1 {
				return fmt.Errorf("usage: suve %s stage diff [name]", cfg.ServiceName)
			}
			if cmd.Args().Len() == 1 {
				strat := cfg.ParserFactory()
				parsedName, err := strat.ParseName(cmd.Args().First())
				if err != nil {
					return err
				}
				name = parsedName
			}

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			strat, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			opts := DiffOptions{
				Name:       name,
				JSONFormat: cmd.Bool("json"),
				NoPager:    cmd.Bool("no-pager"),
			}

			return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
				r := &DiffRunner{
					Strategy: strat,
					Store:    store,
					Stdout:   w,
					Stderr:   cmd.Root().ErrWriter,
				}
				return r.Run(ctx, opts)
			})
		},
	}
}

// NewAddCommand creates an add command with the given config.
func NewAddCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     fmt.Sprintf("Create new %s and stage it", cfg.ItemName),
		ArgsUsage: "<name>",
		Description: fmt.Sprintf(`Open an editor to create a new %s value, then stage the change.

If the %s is already staged for creation, edits the staged value.
The new %s will be created in AWS when you run 'suve %s stage push'.

Use 'suve %s stage edit' to modify an existing %s.
Use 'suve %s stage status' to view staged changes.

EXAMPLES:
   suve %s stage add <name>  Create and stage new %s`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName, cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve %s stage add <name>", cfg.ServiceName)
			}

			name := cmd.Args().First()

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			strat := cfg.ParserFactory()

			r := &AddRunner{
				Strategy: strat,
				Store:    store,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}
			return r.Run(ctx, AddOptions{Name: name})
		},
	}
}

// NewEditCommand creates an edit command with the given config.
func NewEditCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     fmt.Sprintf("Edit %s value and stage changes", cfg.ItemName),
		ArgsUsage: "<name>",
		Description: fmt.Sprintf(`Open an editor to modify a %s value, then stage the change.

If the %s is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately push to AWS).

Use 'suve %s stage delete' to stage a %s for deletion.
Use 'suve %s stage push' to apply staged changes to AWS.
Use 'suve %s stage status' to view staged changes.

EXAMPLES:
   suve %s stage edit <name>  Edit and stage %s`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve %s stage edit <name>", cfg.ServiceName)
			}

			name := cmd.Args().First()

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			strat, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			r := &EditRunner{
				Strategy: strat,
				Store:    store,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}
			return r.Run(ctx, EditOptions{Name: name})
		},
	}
}

// NewPushCommand creates a push command with the given config.
func NewPushCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     fmt.Sprintf("Apply staged %s changes to AWS", cfg.ItemName),
		ArgsUsage: "[name]",
		Description: fmt.Sprintf(`Apply all staged %s %s changes to AWS.

If a %s name is specified, only that %s's staged changes are applied.
Otherwise, all staged %s %s changes are applied.

After successful push, the staged changes are cleared.

Use 'suve %s stage status' to view staged changes before pushing.

EXAMPLES:
   suve %s stage push           Push all staged %s changes (with confirmation)
   suve %s stage push <name>    Push only the specified %s
   suve %s stage push -y        Push without confirmation`,
			cfg.ServiceName, cfg.ItemName,
			cfg.ItemName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			// Get entries to show what will be pushed
			parser := cfg.ParserFactory()
			service := parser.Service()
			entries, err := store.List(service)
			if err != nil {
				return err
			}

			serviceEntries := entries[service]
			if len(serviceEntries) == 0 {
				yellow := color.New(color.FgYellow).SprintFunc()
				_, _ = fmt.Fprintf(cmd.Root().Writer, "%s No %s changes staged.\n", yellow("!"), parser.ServiceName())
				return nil
			}

			// Filter by name if specified
			opts := PushOptions{}
			if cmd.Args().Len() > 0 {
				opts.Name = cmd.Args().First()
				if _, ok := serviceEntries[opts.Name]; !ok {
					return fmt.Errorf("%s is not staged", opts.Name)
				}
			}

			// Confirm push
			skipConfirm := cmd.Bool("yes")
			prompter := &confirm.Prompter{
				Stdin:  os.Stdin,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			var message string
			if opts.Name != "" {
				message = fmt.Sprintf("Push staged changes for %s to AWS?", opts.Name)
			} else {
				message = fmt.Sprintf("Push %d staged %s change(s) to AWS?", len(serviceEntries), parser.ServiceName())
			}

			confirmed, err := prompter.Confirm(message, skipConfirm)
			if err != nil {
				return err
			}
			if !confirmed {
				return nil
			}

			strat, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			r := &PushRunner{
				Strategy: strat,
				Store:    store,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			return r.Run(ctx, opts)
		},
	}
}

// NewResetCommand creates a reset command with the given config.
func NewResetCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "reset",
		Usage:     fmt.Sprintf("Unstage %s or restore to specific version", cfg.ItemName),
		ArgsUsage: "[spec]",
		Description: fmt.Sprintf(`Remove a %s from staging area or restore to a specific version.

Without a version specifier, the %s is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use 'suve %s stage reset --all' to unstage all %s %ss at once.

VERSION SPECIFIERS:
   <name>          Unstage %s (remove from staging)
   <name>#<ver>    Restore to specific version
   <name>~1        Restore to 1 version ago

EXAMPLES:
   suve %s stage reset <name>              Unstage (remove from staging)
   suve %s stage reset <name>#<ver>        Stage value from specific version
   suve %s stage reset <name>~1            Stage value from previous version
   suve %s stage reset --all               Unstage all %s %ss`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName, cfg.ServiceName, cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ServiceName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: fmt.Sprintf("Unstage all %s %ss", cfg.ServiceName, cfg.ItemName),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			resetAll := cmd.Bool("all")

			if !resetAll && cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve %s stage reset <spec> or suve %s stage reset --all", cfg.ServiceName, cfg.ServiceName)
			}

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			opts := ResetOptions{
				All: resetAll,
			}
			if !resetAll {
				opts.Spec = cmd.Args().First()
			}

			parser := cfg.ParserFactory()

			// Check if version spec is provided (need AWS client for FetchVersion)
			var fetcher VersionFetcher
			if !resetAll && opts.Spec != "" {
				_, hasVersion, err := parser.ParseSpec(opts.Spec)
				if err != nil {
					return err
				}
				if hasVersion {
					strat, err := cfg.Factory(ctx)
					if err != nil {
						return err
					}
					fetcher = strat
				}
			}

			r := &ResetRunner{
				Parser:  parser,
				Fetcher: fetcher,
				Store:   store,
				Stdout:  cmd.Root().Writer,
				Stderr:  cmd.Root().ErrWriter,
			}

			return r.Run(ctx, opts)
		},
	}
}

// NewDeleteCommand creates a delete command with the given config.
func NewDeleteCommand(cfg CommandConfig) *cli.Command {
	strat := cfg.ParserFactory()
	hasDeleteOptions := strat.HasDeleteOptions()

	var flags []cli.Flag
	var description string

	if hasDeleteOptions {
		// SM has delete options
		flags = []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Force immediate deletion without recovery window",
			},
			&cli.IntFlag{
				Name:  "recovery-window",
				Usage: "Number of days before permanent deletion (7-30)",
				Value: 30,
			},
		}
		description = fmt.Sprintf(`Stage a %s for deletion.

The %s will be deleted from AWS when you run 'suve %s stage push'.
Use 'suve %s stage status' to view staged changes.
Use 'suve %s stage reset <name>' to unstage.

RECOVERY WINDOW:
   By default, %ss are scheduled for deletion after a 30-day recovery window.
   During this period, you can restore the %s using 'suve %s restore'.
   Use --force for immediate permanent deletion without recovery.

   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   suve %s stage delete <name>                      Stage with 30-day recovery
   suve %s stage delete --recovery-window 7 <name>  Stage with 7-day recovery
   suve %s stage delete --force <name>              Stage for immediate deletion`,
			cfg.ItemName,
			cfg.ItemName, cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ItemName,
			cfg.ItemName, cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName)
	} else {
		// SSM doesn't have delete options
		description = fmt.Sprintf(`Stage a %s for deletion.

The %s will be deleted from AWS when you run 'suve %s stage push'.
Use 'suve %s stage status' to view staged changes.
Use 'suve %s stage reset <name>' to unstage.

EXAMPLES:
   suve %s stage delete <name>  Stage %s for deletion`,
			cfg.ItemName,
			cfg.ItemName, cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName)
	}

	return &cli.Command{
		Name:        "delete",
		Usage:       fmt.Sprintf("Stage a %s for deletion", cfg.ItemName),
		ArgsUsage:   "<name>",
		Description: description,
		Flags:       flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve %s stage delete <name>", cfg.ServiceName)
			}

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			name := cmd.Args().First()
			service := cfg.ParserFactory().Service()

			entry := staging.Entry{
				Operation: staging.OperationDelete,
				StagedAt:  time.Now(),
			}

			if hasDeleteOptions {
				force := cmd.Bool("force")
				recoveryWindow := cmd.Int("recovery-window")

				// Validate recovery window
				if !force && (recoveryWindow < 7 || recoveryWindow > 30) {
					return fmt.Errorf("recovery window must be between 7 and 30 days")
				}

				entry.DeleteOptions = &staging.DeleteOptions{
					Force:          force,
					RecoveryWindow: recoveryWindow,
				}
			}

			if err := store.Stage(service, name, entry); err != nil {
				return err
			}

			green := color.New(color.FgGreen).SprintFunc()
			if hasDeleteOptions && entry.DeleteOptions != nil {
				if entry.DeleteOptions.Force {
					_, _ = fmt.Fprintf(cmd.Root().Writer, "%s Staged for immediate deletion: %s\n", green("✓"), name)
				} else {
					_, _ = fmt.Fprintf(cmd.Root().Writer, "%s Staged for deletion (%d-day recovery): %s\n", green("✓"), entry.DeleteOptions.RecoveryWindow, name)
				}
			} else {
				_, _ = fmt.Fprintf(cmd.Root().Writer, "%s Staged for deletion: %s\n", green("✓"), name)
			}
			return nil
		},
	}
}
