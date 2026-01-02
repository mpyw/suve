// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/staging"
)

// CommandConfig holds service-specific configuration for building stage commands.
type CommandConfig struct {
	// ServiceName is the service prefix for commands (e.g., "param", "secret").
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
   suve stage %s status              Show all staged %s changes
   suve stage %s status <name>       Show staged change for specific %s
   suve stage %s status -v           Show detailed information`,
			cfg.ServiceName, cfg.ItemName, cfg.ItemName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
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
Otherwise, shows diff for all staged %ss.

EXAMPLES:
   suve stage %s diff              Show diff for all staged %ss
   suve stage %s diff <name>       Show diff for specific %s
   suve stage %s diff -j           Show diff with JSON formatting`,
			cfg.ItemName, cfg.ItemName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
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
				return fmt.Errorf("usage: suve stage %s diff [name]", cfg.ServiceName)
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
		ArgsUsage: "<name> [value]",
		Description: fmt.Sprintf(`Create a new %s value and stage the change.

If value is provided as an argument, uses that value directly.
Otherwise, opens an editor to create the value.

If the %s is already staged for creation, edits the staged value.
The new %s will be created in AWS when you run 'suve stage %s apply'.

Use 'suve stage %s edit' to modify an existing %s.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s add <name>              Open editor to create new %s
   suve stage %s add <name> <value>      Create new %s with given value`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ItemName, cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: fmt.Sprintf("Description for the %s", cfg.ItemName),
			},
			&cli.StringSliceFlag{
				Name:  "tag",
				Usage: "Tag in key=value format (can be specified multiple times)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s add <name> [value]", cfg.ServiceName)
			}

			name := cmd.Args().First()
			var value string
			if cmd.Args().Len() >= 2 {
				value = cmd.Args().Get(1)
			}

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			tags, err := parseTags(cmd.StringSlice("tag"))
			if err != nil {
				return err
			}

			strat := cfg.ParserFactory()

			r := &AddRunner{
				Strategy: strat,
				Store:    store,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}
			return r.Run(ctx, AddOptions{
				Name:        name,
				Value:       value,
				Description: cmd.String("description"),
				Tags:        tags,
			})
		},
	}
}

// NewEditCommand creates an edit command with the given config.
func NewEditCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     fmt.Sprintf("Edit %s value and stage changes", cfg.ItemName),
		ArgsUsage: "<name> [value]",
		Description: fmt.Sprintf(`Modify a %s value and stage the change.

If value is provided as an argument, uses that value directly.
Otherwise, opens an editor to modify the value.

If the %s is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately apply to AWS).

Use 'suve stage %s delete' to stage a %s for deletion.
Use 'suve stage %s apply' to apply staged changes to AWS.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s edit <name>              Open editor to modify %s
   suve stage %s edit <name> <value>      Set %s to given value`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: fmt.Sprintf("Description for the %s", cfg.ItemName),
			},
			&cli.StringSliceFlag{
				Name:  "tag",
				Usage: "Tag in key=value format (can be specified multiple times)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s edit <name> [value]", cfg.ServiceName)
			}

			name := cmd.Args().First()
			var value string
			if cmd.Args().Len() >= 2 {
				value = cmd.Args().Get(1)
			}

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			tags, err := parseTags(cmd.StringSlice("tag"))
			if err != nil {
				return err
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
			return r.Run(ctx, EditOptions{
				Name:        name,
				Value:       value,
				Description: cmd.String("description"),
				Tags:        tags,
			})
		},
	}
}

// NewApplyCommand creates an apply command with the given config.
func NewApplyCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:      "apply",
		Aliases:   []string{"push"},
		Usage:     fmt.Sprintf("Apply staged %s changes to AWS", cfg.ItemName),
		ArgsUsage: "[name]",
		Description: fmt.Sprintf(`Apply all staged %s changes to AWS.

If a %s name is specified, only that %s's staged changes are applied.
Otherwise, all staged %s changes are applied.

After successful apply, the staged changes are cleared.

Use 'suve stage %s status' to view staged changes before applying.

CONFLICT DETECTION:
   Before applying, suve checks for conflicts to prevent lost updates:
   - For new resources: checks if someone else created it after staging
   - For existing resources: checks if it was modified after staging
   Use --ignore-conflicts to force apply despite conflicts.

EXAMPLES:
   suve stage %s apply                      Apply all staged %s changes (with confirmation)
   suve stage %s apply <name>               Apply only the specified %s
   suve stage %s apply --yes                Apply without confirmation
   suve stage %s apply --ignore-conflicts   Apply even if AWS was modified after staging`,
			cfg.ItemName,
			cfg.ItemName, cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "ignore-conflicts",
				Usage: "Apply even if AWS was modified after staging",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			// Get entries to show what will be applied
			parser := cfg.ParserFactory()
			service := parser.Service()
			entries, err := store.List(service)
			if err != nil {
				return err
			}

			serviceEntries := entries[service]
			if len(serviceEntries) == 0 {
				_, _ = fmt.Fprintf(cmd.Root().Writer, "%s No %s changes staged.\n", colors.Warning("!"), parser.ServiceName())
				return nil
			}

			// Filter by name if specified
			opts := ApplyOptions{
				IgnoreConflicts: cmd.Bool("ignore-conflicts"),
			}
			if cmd.Args().Len() > 0 {
				opts.Name = cmd.Args().First()
				if _, ok := serviceEntries[opts.Name]; !ok {
					return fmt.Errorf("%s is not staged", opts.Name)
				}
			}

			// Confirm apply
			skipConfirm := cmd.Bool("yes")
			prompter := &confirm.Prompter{
				Stdin:  os.Stdin,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			var message string
			if opts.Name != "" {
				message = fmt.Sprintf("Apply staged changes for %s to AWS?", opts.Name)
			} else {
				message = fmt.Sprintf("Apply %d staged %s change(s) to AWS?", len(serviceEntries), parser.ServiceName())
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

			r := &ApplyRunner{
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

Use 'suve stage %s reset --all' to unstage all %ss at once.

VERSION SPECIFIERS:
   <name>          Unstage %s (remove from staging)
   <name>#<ver>    Restore to specific version
   <name>~1        Restore to 1 version ago

EXAMPLES:
   suve stage %s reset <name>              Unstage (remove from staging)
   suve stage %s reset <name>#<ver>        Stage value from specific version
   suve stage %s reset <name>~1            Stage value from previous version
   suve stage %s reset --all               Unstage all %ss`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName, cfg.ItemName,
			cfg.ItemName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName,
			cfg.ServiceName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: fmt.Sprintf("Unstage all %ss", cfg.ItemName),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			resetAll := cmd.Bool("all")

			if !resetAll && cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s reset <spec> or suve stage %s reset --all", cfg.ServiceName, cfg.ServiceName)
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

func parseTags(tagSlice []string) (map[string]string, error) {
	if len(tagSlice) == 0 {
		return nil, nil
	}
	tags := make(map[string]string)
	for _, t := range tagSlice {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", t)
		}
		tags[parts[0]] = parts[1]
	}
	return tags, nil
}

// NewDeleteCommand creates a delete command with the given config.
func NewDeleteCommand(cfg CommandConfig) *cli.Command {
	strat := cfg.ParserFactory()
	hasDeleteOptions := strat.HasDeleteOptions()

	var flags []cli.Flag
	var description string

	if hasDeleteOptions {
		// Secrets Manager has delete options
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

The %s will be deleted from AWS when you run 'suve stage %s apply'.
Use 'suve stage %s status' to view staged changes.
Use 'suve stage %s reset <name>' to unstage.

RECOVERY WINDOW:
   By default, %ss are scheduled for deletion after a 30-day recovery window.
   During this period, you can restore the %s using 'suve %s restore'.
   Use --force for immediate permanent deletion without recovery.

   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   suve stage %s delete <name>                      Stage with 30-day recovery
   suve stage %s delete --recovery-window 7 <name>  Stage with 7-day recovery
   suve stage %s delete --force <name>              Stage for immediate deletion`,
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
		// SSM Parameter Store doesn't have delete options
		description = fmt.Sprintf(`Stage a %s for deletion.

The %s will be deleted from AWS when you run 'suve stage %s apply'.
Use 'suve stage %s status' to view staged changes.
Use 'suve stage %s reset <name>' to unstage.

EXAMPLES:
   suve stage %s delete <name>  Stage %s for deletion`,
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
				return fmt.Errorf("usage: suve stage %s delete <name>", cfg.ServiceName)
			}

			store, err := staging.NewStore()
			if err != nil {
				return fmt.Errorf("failed to initialize stage store: %w", err)
			}

			name := cmd.Args().First()

			// Initialize strategy to fetch LastModified for conflict detection
			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}
			service := strategy.Service()

			// Fetch LastModified for conflict detection
			lastModified, err := strategy.FetchLastModified(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to fetch %s: %w", cfg.ItemName, err)
			}

			entry := staging.Entry{
				Operation: staging.OperationDelete,
				StagedAt:  time.Now(),
			}
			if !lastModified.IsZero() {
				entry.BaseModifiedAt = &lastModified
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

			if hasDeleteOptions && entry.DeleteOptions != nil {
				if entry.DeleteOptions.Force {
					_, _ = fmt.Fprintf(cmd.Root().Writer, "%s Staged for immediate deletion: %s\n", colors.Success("✓"), name)
				} else {
					_, _ = fmt.Fprintf(cmd.Root().Writer, "%s Staged for deletion (%d-day recovery): %s\n", colors.Success("✓"), entry.DeleteOptions.RecoveryWindow, name)
				}
			} else {
				_, _ = fmt.Fprintf(cmd.Root().Writer, "%s Staged for deletion: %s\n", colors.Success("✓"), name)
			}
			return nil
		},
	}
}
