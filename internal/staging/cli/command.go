// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// CommandConfig holds service-specific configuration for building stage commands.
type CommandConfig struct {
	// CommandName is the subcommand name (e.g., "param", "secret").
	CommandName string

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

Use --verbose to show detailed information including the staged value.

EXAMPLES:
   suve stage %s status              Show all staged %s changes
   suve stage %s status <name>       Show staged change for specific %s
   suve stage %s status --verbose    Show detailed information`,
			cfg.CommandName, cfg.ItemName, cfg.ItemName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)

			opts := StatusOptions{
				Verbose: cmd.Bool("verbose"),
			}
			if cmd.Args().Len() > 0 {
				opts.Name = cmd.Args().First()
			}

			result, err := lifecycle.ExecuteRead(ctx, store, lifecycle.CmdStatus, func() (struct{}, error) {
				r := &StatusRunner{
					UseCase: &stagingusecase.StatusUseCase{
						Strategy: cfg.ParserFactory(),
						Store:    store,
					},
					Stdout: cmd.Root().Writer,
					Stderr: cmd.Root().ErrWriter,
				}

				return struct{}{}, r.Run(ctx, opts)
			})
			if err != nil {
				return err
			}

			if result.NothingStaged {
				output.Info(cmd.Root().Writer, "No %s changes staged.", cfg.ItemName)
			}

			return nil
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
   suve stage %s diff                   Show diff for all staged %ss
   suve stage %s diff <name>            Show diff for specific %s
   suve stage %s diff --parse-json      Show diff with JSON formatting`,
			cfg.ItemName, cfg.ItemName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
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
				return fmt.Errorf("usage: suve stage %s diff [name]", cfg.CommandName)
			}

			if cmd.Args().Len() == 1 {
				parser := cfg.ParserFactory()

				parsedName, err := parser.ParseName(cmd.Args().First())
				if err != nil {
					return err
				}

				name = parsedName
			}

			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)

			opts := DiffOptions{
				Name:      name,
				ParseJSON: cmd.Bool("parse-json"),
				NoPager:   cmd.Bool("no-pager"),
			}

			result, err := lifecycle.ExecuteRead(ctx, store, lifecycle.CmdDiff, func() (struct{}, error) {
				strategy, err := cfg.Factory(ctx)
				if err != nil {
					return struct{}{}, err
				}

				return struct{}{}, pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
					r := &DiffRunner{
						UseCase: &stagingusecase.DiffUseCase{
							Strategy: strategy,
							Store:    store,
						},
						Stdout: w,
						Stderr: cmd.Root().ErrWriter,
					}

					return r.Run(ctx, opts)
				})
			})
			if err != nil {
				return err
			}

			if result.NothingStaged {
				output.Warning(cmd.Root().ErrWriter, "nothing staged")
			}

			return nil
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
			cfg.ItemName, cfg.CommandName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: fmt.Sprintf("Description for the %s", cfg.ItemName),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s add <name> [value]", cfg.CommandName)
			}

			name := cmd.Args().First()

			var value string
			if cmd.Args().Len() >= 2 { //nolint:mnd // check for optional value argument
				value = cmd.Args().Get(1)
			}

			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return fmt.Errorf("failed to initialize strategy: %w", err)
			}

			r := &AddRunner{
				UseCase: &stagingusecase.AddUseCase{
					Strategy: strategy,
					Store:    store,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, AddOptions{
				Name:        name,
				Value:       value,
				Description: cmd.String("description"),
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
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: fmt.Sprintf("Description for the %s", cfg.ItemName),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s edit <name> [value]", cfg.CommandName)
			}

			name := cmd.Args().First()

			var value string
			if cmd.Args().Len() >= 2 { //nolint:mnd // check for optional value argument
				value = cmd.Args().Get(1)
			}

			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			r := &EditRunner{
				UseCase: &stagingusecase.EditUseCase{
					Strategy: strategy,
					Store:    store,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, EditOptions{
				Name:        name,
				Value:       value,
				Description: cmd.String("description"),
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
			cfg.CommandName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName, cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName),
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
			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)
			parser := cfg.ParserFactory()

			result, err := lifecycle.ExecuteRead(ctx, store, lifecycle.CmdApply, func() (struct{}, error) {
				// Get entries to show what will be applied
				service := parser.Service()

				entries, err := store.ListEntries(ctx, service)
				if err != nil {
					return struct{}{}, err
				}

				serviceEntries := entries[service]
				if len(serviceEntries) == 0 {
					output.Info(cmd.Root().Writer, "No %s changes staged.", parser.ServiceName())

					return struct{}{}, nil
				}

				// Filter by name if specified
				opts := ApplyOptions{
					IgnoreConflicts: cmd.Bool("ignore-conflicts"),
				}
				if cmd.Args().Len() > 0 {
					opts.Name = cmd.Args().First()
					if _, ok := serviceEntries[opts.Name]; !ok {
						return struct{}{}, fmt.Errorf("%s is not staged", opts.Name)
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
					return struct{}{}, err
				}

				if !confirmed {
					return struct{}{}, nil
				}

				strategy, err := cfg.Factory(ctx)
				if err != nil {
					return struct{}{}, err
				}

				r := &ApplyRunner{
					UseCase: &stagingusecase.ApplyUseCase{
						Strategy: strategy,
						Store:    store,
					},
					Stdout: cmd.Root().Writer,
					Stderr: cmd.Root().ErrWriter,
				}

				return struct{}{}, r.Run(ctx, opts)
			})
			if err != nil {
				return err
			}

			if result.NothingStaged {
				output.Info(cmd.Root().Writer, "No %s changes staged.", parser.ServiceName())
			}

			return nil
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
			cfg.CommandName, cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName, cfg.ItemName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: fmt.Sprintf("Unstage all %ss", cfg.ItemName),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			resetAll := cmd.Bool("all")

			if !resetAll && cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s reset <spec> or suve stage %s reset --all", cfg.CommandName, cfg.CommandName)
			}

			opts := ResetOptions{
				All: resetAll,
			}

			if !resetAll {
				opts.Spec = cmd.Args().First()
			}

			parser := cfg.ParserFactory()

			// Check if version spec is provided before choosing the execution path
			// If version spec exists, this is a write operation that should auto-start the agent
			var hasVersion bool

			if !resetAll && opts.Spec != "" {
				var err error

				_, hasVersion, err = parser.ParseSpec(opts.Spec)
				if err != nil {
					return err
				}
			}

			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)

			if hasVersion {
				// Reset with version spec - write operation, auto-start the agent
				_, err := lifecycle.ExecuteWrite(ctx, store, lifecycle.CmdResetVersion, func() (struct{}, error) {
					strategy, err := cfg.Factory(ctx)
					if err != nil {
						return struct{}{}, err
					}

					r := &ResetRunner{
						UseCase: &stagingusecase.ResetUseCase{
							Parser:  parser,
							Fetcher: strategy,
							Store:   store,
						},
						Stdout: cmd.Root().Writer,
						Stderr: cmd.Root().ErrWriter,
					}

					return struct{}{}, r.Run(ctx, opts)
				})

				return err
			}

			// Reset without version spec - read operation, check if agent is running
			result, err := lifecycle.ExecuteRead(ctx, store, lifecycle.CmdReset, func() (struct{}, error) {
				r := &ResetRunner{
					UseCase: &stagingusecase.ResetUseCase{
						Parser:  parser,
						Fetcher: nil,
						Store:   store,
					},
					Stdout: cmd.Root().Writer,
					Stderr: cmd.Root().ErrWriter,
				}

				return struct{}{}, r.Run(ctx, opts)
			})
			if err != nil {
				return err
			}

			if result.NothingStaged {
				output.Info(cmd.Root().Writer, "No %s changes staged.", cfg.ItemName)
			}

			return nil
		},
	}
}

// NewDeleteCommand creates a delete command with the given config.
func NewDeleteCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	hasDeleteOptions := parser.HasDeleteOptions()

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
				Value: 30, //nolint:mnd // AWS Secrets Manager default recovery window
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
			cfg.ItemName, cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.ItemName, cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName)
	} else {
		// SSM Parameter Store doesn't have delete options
		description = fmt.Sprintf(`Stage a %s for deletion.

The %s will be deleted from AWS when you run 'suve stage %s apply'.
Use 'suve stage %s status' to view staged changes.
Use 'suve stage %s reset <name>' to unstage.

EXAMPLES:
   suve stage %s delete <name>  Stage %s for deletion`,
			cfg.ItemName,
			cfg.ItemName, cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName, cfg.ItemName)
	}

	return &cli.Command{
		Name:        "delete",
		Usage:       fmt.Sprintf("Stage a %s for deletion", cfg.ItemName),
		ArgsUsage:   "<name>",
		Description: description,
		Flags:       flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s delete <name>", cfg.CommandName)
			}

			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			store := agent.NewStore(identity.AccountID, identity.Region)

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			name := cmd.Args().First()
			force := cmd.Bool("force")
			recoveryWindow := cmd.Int("recovery-window")

			r := &DeleteRunner{
				UseCase: &stagingusecase.DeleteUseCase{
					Strategy: strategy,
					Store:    store,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, DeleteOptions{
				Name:           name,
				Force:          force,
				RecoveryWindow: recoveryWindow,
			})
		},
	}
}

// tagCommandRunner is a function that runs a tag or untag command.
type tagCommandRunner func(
	ctx context.Context,
	useCase *stagingusecase.TagUseCase,
	stdout, stderr io.Writer,
	name string,
	args []string,
) error

// tagAction creates a common action handler for tag/untag commands.
func tagAction(cfg CommandConfig, usageMsg string, runner tagCommandRunner) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and key/value
			return fmt.Errorf("usage: suve stage %s %s", cfg.CommandName, usageMsg)
		}

		name := cmd.Args().First()
		args := cmd.Args().Slice()[1:]

		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		store := agent.NewStore(identity.AccountID, identity.Region)

		strategy, err := cfg.Factory(ctx)
		if err != nil {
			return err
		}

		useCase := &stagingusecase.TagUseCase{
			Strategy: strategy,
			Store:    store,
		}

		return runner(ctx, useCase, cmd.Root().Writer, cmd.Root().ErrWriter, name, args)
	}
}

// NewTagCommand creates a tag command with the given config.
func NewTagCommand(cfg CommandConfig) *cli.Command {
	runner := func(
		ctx context.Context,
		useCase *stagingusecase.TagUseCase,
		stdout, stderr io.Writer,
		name string,
		tags []string,
	) error {
		r := &TagRunner{
			UseCase: useCase,
			Stdout:  stdout,
			Stderr:  stderr,
		}

		return r.Run(ctx, TagOptions{Name: name, Tags: tags})
	}

	return &cli.Command{
		Name:      "tag",
		Usage:     fmt.Sprintf("Stage tags for a %s", cfg.ItemName),
		ArgsUsage: "<name> <key>=<value>...",
		Description: fmt.Sprintf(`Stage tags to add or update for a %s.

Tags are staged locally and applied when you run 'suve stage %s apply'.
If the %s is not already staged, a tag-only change is created.

Use 'suve stage %s untag' to stage tag removals.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s tag <name> env=prod              Stage single tag
   suve stage %s tag <name> env=prod team=api     Stage multiple tags`,
			cfg.ItemName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Action: tagAction(cfg, "tag <name> <key>=<value>", runner),
	}
}

// NewUntagCommand creates an untag command with the given config.
func NewUntagCommand(cfg CommandConfig) *cli.Command {
	runner := func(
		ctx context.Context,
		useCase *stagingusecase.TagUseCase,
		stdout, stderr io.Writer,
		name string,
		keys []string,
	) error {
		r := &UntagRunner{
			UseCase: useCase,
			Stdout:  stdout,
			Stderr:  stderr,
		}

		return r.Run(ctx, UntagOptions{Name: name, Keys: keys})
	}

	return &cli.Command{
		Name:      "untag",
		Usage:     fmt.Sprintf("Stage tag removal for a %s", cfg.ItemName),
		ArgsUsage: "<name> <key>...",
		Description: fmt.Sprintf(`Stage tags to remove from a %s.

Tag removals are staged locally and applied when you run 'suve stage %s apply'.
If the %s is not already staged, a tag-only change is created.

Use 'suve stage %s tag' to stage tag additions.
Use 'suve stage %s status' to view staged changes.

EXAMPLES:
   suve stage %s untag <name> env              Stage single tag removal
   suve stage %s untag <name> env team         Stage multiple tag removals`,
			cfg.ItemName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Action: tagAction(cfg, "untag <name> <key>", runner),
	}
}
