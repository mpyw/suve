// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// Flag names, flag usages, command names, and arg-usage strings shared across
// sibling stage command builders.
const (
	flagYes                = "yes"
	usageSkipConfirm       = "Skip confirmation prompt"
	flagPassphraseStdin    = "passphrase-stdin"
	usagePassphraseStdin   = "Read passphrase from stdin (for scripts/automation)"
	flagMerge              = "merge"
	flagOverwrite          = "overwrite"
	flagForce              = "force"
	flagAllowScopeMismatch = "allow-scope-mismatch"
	cmdNamePush            = "push"
	argsUsageName          = "[name]"
)

// CommandConfig holds service-specific configuration for building stage commands.
type CommandConfig struct {
	// CommandName is the subcommand name (e.g., "param", "secret").
	CommandName string

	// ItemName is the item name for messages (e.g., "parameter", "secret").
	ItemName string

	// Factory creates a FullStrategy backed by a provider.Store.
	Factory staging.StrategyFactory

	// ParserFactory creates a Parser without provider access (for status, parsing).
	ParserFactory staging.ParserFactory

	// ScopeResolver resolves the provider staging scope used to key on-disk
	// state. When nil, it defaults to AWSScopeResolver, preserving AWS behavior.
	ScopeResolver staging.ScopeResolver

	// Namespace resolves the App Configuration namespace a single-item staging
	// op targets, from the command context (the --namespace flag). It records
	// the namespace on the staged entry as part of its identity. Nil for
	// providers without a namespace axis (the namespace is then always empty).
	Namespace func(ctx context.Context) string

	// StrategyForNamespace builds a strategy backed by a provider store scoped to
	// the given namespace, so status/diff/apply act on each staged entry under
	// its own namespace (App Configuration keeps all namespaces in one staging
	// store). Nil for providers without a namespace axis.
	StrategyForNamespace func(ctx context.Context, namespace string) (staging.FullStrategy, error)

	// ValueTypeFlags are provider-specific flags appended to the add and edit
	// commands so the staged entry can carry a value type (the AWS SSM Parameter
	// Store --type/--secure axis). Nil for providers without a value-type axis.
	ValueTypeFlags []cli.Flag

	// ValueTypeFromCmd validates and resolves the staged value type from the
	// add/edit command flags (ValueTypeFlags). Nil for providers without a
	// value-type axis, in which case the staged value type is left unset. An
	// empty return means "not specified": create applies plaintext and update
	// preserves the existing type.
	ValueTypeFromCmd func(cmd *cli.Command) (domain.ValueType, error)
}

// valueTypeFor resolves the staged value type from the command flags, or ""
// when the provider has no value-type axis.
func (c CommandConfig) valueTypeFor(cmd *cli.Command) (domain.ValueType, error) {
	if c.ValueTypeFromCmd == nil {
		return "", nil
	}

	return c.ValueTypeFromCmd(cmd)
}

// namespaceFor returns the single-item namespace for this command context, or ""
// for providers without a namespace axis.
func (c CommandConfig) namespaceFor(ctx context.Context) string {
	if c.Namespace == nil {
		return ""
	}

	return c.Namespace(ctx)
}

// diffStrategyFor adapts StrategyForNamespace to the DiffUseCase resolver, or nil
// when the provider has no namespace axis (the single strategy handles all).
func (c CommandConfig) diffStrategyFor(ctx context.Context) func(string) (staging.DiffStrategy, error) {
	if c.StrategyForNamespace == nil {
		return nil
	}

	return func(ns string) (staging.DiffStrategy, error) {
		return c.StrategyForNamespace(ctx, ns)
	}
}

// applyStrategyFor adapts StrategyForNamespace to the ApplyUseCase resolver, or
// nil when the provider has no namespace axis.
func (c CommandConfig) applyStrategyFor(ctx context.Context) func(string) (staging.ApplyStrategy, error) {
	if c.StrategyForNamespace == nil {
		return nil
	}

	return func(ns string) (staging.ApplyStrategy, error) {
		return c.StrategyForNamespace(ctx, ns)
	}
}

// NewStatusCommand creates a status command with the given config.
func NewStatusCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "status",
		Usage:       fmt.Sprintf("Show staged %s changes", cfg.ItemName),
		ArgsUsage:   argsUsageName,
		Description: statusDescription(cfg),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, _, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			opts := StatusOptions{
				Verbose: cmd.Bool("verbose"),
			}
			if cmd.Args().Len() > 0 {
				opts.Name = cmd.Args().First()
			}

			r := &StatusRunner{
				UseCase: &stagingusecase.StatusUseCase{
					Strategy: cfg.ParserFactory(),
					Store:    store,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, opts)
		},
	}
}

// NewDiffCommand creates a diff command with the given config.
func NewDiffCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "diff",
		Usage:       "Show diff between staged and AWS values",
		ArgsUsage:   argsUsageName,
		Description: diffDescription(cfg),
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

			store, _, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			opts := DiffOptions{
				Name:      name,
				ParseJSON: cmd.Bool("parse-json"),
				NoPager:   cmd.Bool("no-pager"),
			}

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
				r := &DiffRunner{
					UseCase: &stagingusecase.DiffUseCase{
						Strategy:    strategy,
						Store:       store,
						StrategyFor: cfg.diffStrategyFor(ctx),
					},
					Stdout: w,
					Stderr: cmd.Root().ErrWriter,
				}

				return r.Run(ctx, opts)
			})
		},
	}
}

// NewAddCommand creates an add command with the given config.
func NewAddCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "add",
		Usage:       fmt.Sprintf("Create new %s and stage it", cfg.ItemName),
		ArgsUsage:   "<name> [value]",
		Description: addDescription(cfg),
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: fmt.Sprintf("Description for the %s", cfg.ItemName),
			},
		}, cfg.ValueTypeFlags...),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s add <name> [value]", cfg.CommandName)
			}

			name := cmd.Args().First()

			var value string
			if cmd.Args().Len() >= 2 { //nolint:mnd // check for optional value argument
				value = cmd.Args().Get(1)
			}

			valueType, err := cfg.valueTypeFor(cmd)
			if err != nil {
				return err
			}

			store, _, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

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
				Namespace:   cfg.namespaceFor(ctx),
				ValueType:   valueType,
			})
		},
	}
}

// NewEditCommand creates an edit command with the given config.
func NewEditCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "edit",
		Usage:       fmt.Sprintf("Edit %s value and stage changes", cfg.ItemName),
		ArgsUsage:   "<name> [value]",
		Description: editDescription(cfg),
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: fmt.Sprintf("Description for the %s", cfg.ItemName),
			},
		}, cfg.ValueTypeFlags...),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s edit <name> [value]", cfg.CommandName)
			}

			name := cmd.Args().First()

			var value string
			if cmd.Args().Len() >= 2 { //nolint:mnd // check for optional value argument
				value = cmd.Args().Get(1)
			}

			valueType, err := cfg.valueTypeFor(cmd)
			if err != nil {
				return err
			}

			store, _, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

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
				Namespace:   cfg.namespaceFor(ctx),
				ValueType:   valueType,
			})
		},
	}
}

// NewApplyCommand creates an apply command with the given config.
func NewApplyCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "apply",
		Aliases:     []string{cmdNamePush},
		Usage:       fmt.Sprintf("Apply staged %s changes to AWS", cfg.ItemName),
		ArgsUsage:   argsUsageName,
		Description: applyDescription(cfg),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  flagYes,
				Usage: usageSkipConfirm,
			},
			&cli.BoolFlag{
				Name:  "ignore-conflicts",
				Usage: "Apply even if AWS was modified after staging",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, resolved, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			opts := ApplyOptions{
				IgnoreConflicts: cmd.Bool("ignore-conflicts"),
			}
			if cmd.Args().Len() > 0 {
				opts.Name = cmd.Args().First()
			}

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			prompter := &confirm.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
				Target: resolved.Target,
			}

			r := &ApplyRunner{
				UseCase: &stagingusecase.ApplyUseCase{
					Strategy:    strategy,
					Store:       store,
					StrategyFor: cfg.applyStrategyFor(ctx),
				},
				Store:       store,
				Parser:      cfg.ParserFactory(),
				Confirmer:   prompter,
				SkipConfirm: cmd.Bool(flagYes),
				Stdout:      cmd.Root().Writer,
				Stderr:      cmd.Root().ErrWriter,
			}

			return r.RunInteractive(ctx, opts)
		},
	}
}

// NewResetCommand creates a reset command with the given config.
func NewResetCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "reset",
		Usage:       fmt.Sprintf("Unstage %s or restore to specific version", cfg.ItemName),
		ArgsUsage:   "[spec]",
		Description: resetDescription(cfg),
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
				All:       resetAll,
				Namespace: cfg.namespaceFor(ctx),
			}

			if !resetAll {
				opts.Spec = cmd.Args().First()
			}

			parser := cfg.ParserFactory()

			// Check if a version spec is provided. If so, we need a fetcher strategy
			// to restore the value from AWS.
			var hasVersion bool

			if !resetAll && opts.Spec != "" {
				var err error

				_, hasVersion, err = parser.ParseSpec(opts.Spec)
				if err != nil {
					return err
				}
			}

			store, _, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			var fetcher staging.ResetStrategy

			if hasVersion {
				strategy, err := cfg.Factory(ctx)
				if err != nil {
					return err
				}

				fetcher = strategy
			}

			r := &ResetRunner{
				UseCase: &stagingusecase.ResetUseCase{
					Parser:  parser,
					Fetcher: fetcher,
					Store:   store,
				},
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			return r.Run(ctx, opts)
		},
	}
}

// NewDeleteCommand creates a delete command with the given config.
func NewDeleteCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	hasDeleteOptions := parser.HasDeleteOptions()

	var flags []cli.Flag

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
	}

	return &cli.Command{
		Name:        "delete",
		Usage:       fmt.Sprintf("Stage a %s for deletion", cfg.ItemName),
		ArgsUsage:   "<name>",
		Description: deleteDescription(cfg, hasDeleteOptions),
		Flags:       flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: suve stage %s delete <name>", cfg.CommandName)
			}

			store, _, err := workingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

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
				Namespace:      cfg.namespaceFor(ctx),
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

		store, _, err := workingStore(ctx, cfg.ScopeResolver)
		if err != nil {
			return err
		}

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

		return r.Run(ctx, TagOptions{Name: name, Namespace: cfg.namespaceFor(ctx), Tags: tags})
	}

	return &cli.Command{
		Name:        "tag",
		Usage:       fmt.Sprintf("Stage tags for a %s", cfg.ItemName),
		ArgsUsage:   "<name> <key>=<value>...",
		Description: tagDescription(cfg),
		Action:      tagAction(cfg, "tag <name> <key>=<value>", runner),
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

		return r.Run(ctx, UntagOptions{Name: name, Namespace: cfg.namespaceFor(ctx), Keys: keys})
	}

	return &cli.Command{
		Name:        "untag",
		Usage:       fmt.Sprintf("Stage tag removal for a %s", cfg.ItemName),
		ArgsUsage:   "<name> <key>...",
		Description: untagDescription(cfg),
		Action:      tagAction(cfg, "untag <name> <key>", runner),
	}
}
