// The shared command-building machinery (CommandConfig, the runners and
// builders) is the unit this package is named for; a "command" prefix on these
// names would only stutter at the cli.X call sites.
//declscope:core

// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// Flag names, flag usages, command names, and arg-usage strings shared across
// sibling stage command builders.
//
//declscope:package // shared by design with the per-command files (export.go, import.go, ...)
const (
	flagYes                = "yes"
	usageSkipConfirm       = "Skip confirmation prompt"
	flagPassphraseStdin    = "passphrase-stdin"
	usagePassphraseStdin   = "Read passphrase from stdin (for scripts/automation)"
	flagMerge              = "merge"
	flagOverwrite          = "overwrite"
	flagForce              = "force"
	flagAllowScopeMismatch = "allow-scope-mismatch"
	//declscope:private
	cmdNamePush = "push"
	//declscope:private
	argsUsageName = "[name]"
)

// CommandConfig holds service-specific configuration for building stage commands.
type CommandConfig struct {
	// CommandName is the subcommand name (e.g., "param", "secret").
	CommandName string

	// ItemName is the item name for messages (e.g., "parameter", "secret").
	ItemName string

	// ProviderLabel is the human-readable provider name used in help text,
	// prompts, and messages (e.g., "AWS", "Google Cloud", "Azure").
	ProviderLabel string

	// CommandPath is the explicit command path of this service's stage group,
	// used in help examples and usage errors (e.g., "suve aws stage param",
	// "suve gcloud stage", "suve azure stage secret").
	CommandPath string

	// Factory creates a FullStrategy backed by a provider.Store.
	Factory staging.StrategyFactory

	// ParserFactory creates a Parser without provider access (for status, parsing).
	ParserFactory staging.ParserFactory

	// ScopeResolver resolves the provider staging scope used to key on-disk
	// state. Required: a nil resolver makes every staging command fail.
	ScopeResolver staging.ScopeResolver

	// HasDescription reports whether this service's writer honors a free-text
	// description (AWS param/secret and Google Cloud secret). When true, `stage
	// add`/`stage edit` register a --description flag; when false the flag is not
	// registered at all, so a description on an unsupported provider (Azure Key
	// Vault / App Configuration) is rejected as an unknown flag rather than being
	// accepted and then silently dropped on apply.
	HasDescription bool

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
	// commands so the staged entry can carry a value type (the AWS Parameter
	// Store --type/--secure axis). Nil for providers without a value-type axis.
	ValueTypeFlags []cli.Flag

	// ValueTypeFromCmd validates and resolves the staged value type from the
	// add/edit command flags (ValueTypeFlags). Nil for providers without a
	// value-type axis, in which case the staged value type is left unset. An
	// empty return means "not specified": create applies plaintext and update
	// preserves the existing type.
	ValueTypeFromCmd func(cmd *cli.Command) (domain.ValueType, error)
}

// remoteName is the provider label used in messages, or "remote" when unset.
//
//declscope:package // shared by design with the apply/edit runners
func remoteName(providerLabel string) string {
	if providerLabel == "" {
		return "remote"
	}

	return providerLabel
}

// valueTypeFor resolves the staged value type from the command flags, or ""
// when the provider has no value-type axis.
func (c CommandConfig) valueTypeFor(cmd *cli.Command) (domain.ValueType, error) {
	if c.ValueTypeFromCmd == nil {
		return "", nil
	}

	return c.ValueTypeFromCmd(cmd)
}

// descriptionFlags returns the --description flag for add/edit when the service
// honors a description, or an empty slice otherwise (so an unsupported provider
// rejects --description as an unknown flag rather than silently dropping it).
func (c CommandConfig) descriptionFlags() []cli.Flag {
	if !c.HasDescription {
		return nil
	}

	return []cli.Flag{
		&cli.StringFlag{
			Name:  "description",
			Usage: fmt.Sprintf("Description for the %s", c.ItemName),
		},
	}
}

// description reads the --description flag value; it is always "" when the flag
// is not registered (unsupported provider).
func (c CommandConfig) description(cmd *cli.Command) string {
	if !c.HasDescription {
		return ""
	}

	return cmd.String("description")
}

// namespaceFor returns the single-item namespace for this command context, or ""
// for providers without a namespace axis.
func (c CommandConfig) namespaceFor(ctx context.Context) string {
	if c.Namespace == nil {
		return ""
	}

	return c.Namespace(ctx)
}

// diffStrategyFor adapts a StrategyForNamespace builder to the DiffUseCase
// resolver, or nil when the service has no namespace axis (the single strategy
// handles all).
//
//declscope:package // shared with the all-service diff (global_diff.go)
func diffStrategyFor(
	ctx context.Context, forNamespace func(context.Context, string) (staging.FullStrategy, error),
) func(string) (staging.DiffStrategy, error) {
	if forNamespace == nil {
		return nil
	}

	return func(ns string) (staging.DiffStrategy, error) {
		return forNamespace(ctx, ns)
	}
}

// applyStrategyFor adapts a StrategyForNamespace builder to the ApplyUseCase
// resolver, or nil when the service has no namespace axis.
//
//declscope:package // shared with the all-service apply (global_apply.go)
func applyStrategyFor(
	ctx context.Context, forNamespace func(context.Context, string) (staging.FullStrategy, error),
) func(string) (staging.ApplyStrategy, error) {
	if forNamespace == nil {
		return nil
	}

	return func(ns string) (staging.ApplyStrategy, error) {
		return forNamespace(ctx, ns)
	}
}

// NewStatusCommand creates a status command with the given config.
func NewStatusCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "status",
		Usage:       fmt.Sprintf("Show staged %s changes", cfg.ItemName),
		ArgsUsage:   argsUsageName,
		Description: statusHelp(cfg),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			opts := statusOptions{
				verbose: cmd.Bool("verbose"),
			}
			if cmd.Args().Len() > 0 {
				opts.name = cmd.Args().First()
			}

			r := &statusRunner{
				useCase: &stagingusecase.StatusUseCase{
					Strategy: cfg.ParserFactory(),
					Store:    store,
				},
				stdout: cmd.Root().Writer,
				stderr: cmd.Root().ErrWriter,
			}

			return r.run(ctx, opts)
		},
	}
}

// NewDiffCommand creates a diff command with the given config.
func NewDiffCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "diff",
		Usage:       fmt.Sprintf("Show diff between staged and %s values", remoteName(cfg.ProviderLabel)),
		ArgsUsage:   argsUsageName,
		Description: diffHelp(cfg),
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
				return fmt.Errorf("usage: %s diff [name]", cfg.CommandPath)
			}

			if cmd.Args().Len() == 1 {
				parser := cfg.ParserFactory()

				parsedName, err := parser.ParseName(cmd.Args().First())
				if err != nil {
					return err
				}

				name = parsedName
			}

			store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			opts := diffOptions{
				name:      name,
				parseJSON: cmd.Bool("parse-json"),
				noPager:   cmd.Bool("no-pager"),
			}

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			return pager.WithPagerWriter(cmd.Root().Writer, opts.noPager, func(w io.Writer) error {
				r := &diffRunner{
					useCase: &stagingusecase.DiffUseCase{
						Strategy:    strategy,
						Store:       store,
						StrategyFor: diffStrategyFor(ctx, cfg.StrategyForNamespace),
					},
					stdout: w,
					stderr: cmd.Root().ErrWriter,
				}

				return r.run(ctx, opts)
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
		Description: addHelp(cfg),
		// The --description flag is gated on HasDescription (#666: unsupported
		// providers reject it rather than silently drop it); value-type flags are
		// appended for providers with a value-type axis (AWS Parameter Store, #664).
		Flags: append(append(cfg.descriptionFlags(), valueinput.ValueStdinFlag()), cfg.ValueTypeFlags...),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: %s add <name> [value]", cfg.CommandPath)
			}

			name := cmd.Args().First()
			// Count the arguments rather than test the value, so an explicit ""
			// is a value and not a request for the editor.
			hasValue := cmd.Args().Len() >= 2 //nolint:mnd // optional value argument
			value := cmd.Args().Get(1)

			valueType, err := cfg.valueTypeFor(cmd)
			if err != nil {
				return err
			}

			store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return fmt.Errorf("failed to initialize strategy: %w", err)
			}

			r := &addRunner{
				useCase: &stagingusecase.AddUseCase{
					Strategy: strategy,
					Store:    store,
				},
				stdout: cmd.Root().Writer,
				stderr: cmd.Root().ErrWriter,
				stdin:  valueinput.ValueStdin(cmd),
			}

			return r.run(ctx, addOptions{
				name:           name,
				value:          value,
				hasValue:       hasValue,
				valueFromStdin: cmd.Bool(valueinput.FlagValueStdin),
				description:    cfg.description(cmd),
				namespace:      cfg.namespaceFor(ctx),
				valueType:      valueType,
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
		Description: editHelp(cfg),
		// The --description flag is gated on HasDescription (#666: unsupported
		// providers reject it rather than silently drop it); value-type flags are
		// appended for providers with a value-type axis (AWS Parameter Store, #664).
		Flags: append(append(cfg.descriptionFlags(), valueinput.ValueStdinFlag()), cfg.ValueTypeFlags...),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: %s edit <name> [value]", cfg.CommandPath)
			}

			name := cmd.Args().First()
			// Count the arguments rather than test the value, so an explicit ""
			// is a value and not a request for the editor.
			hasValue := cmd.Args().Len() >= 2 //nolint:mnd // optional value argument
			value := cmd.Args().Get(1)

			valueType, err := cfg.valueTypeFor(cmd)
			if err != nil {
				return err
			}

			store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			r := &editRunner{
				useCase: &stagingusecase.EditUseCase{
					Strategy: strategy,
					Store:    store,
				},
				providerLabel: cfg.ProviderLabel,
				stdout:        cmd.Root().Writer,
				stderr:        cmd.Root().ErrWriter,
				stdin:         valueinput.ValueStdin(cmd),
			}

			return r.run(ctx, editOptions{
				name:           name,
				value:          value,
				hasValue:       hasValue,
				valueFromStdin: cmd.Bool(valueinput.FlagValueStdin),
				description:    cfg.description(cmd),
				namespace:      cfg.namespaceFor(ctx),
				valueType:      valueType,
			})
		},
	}
}

// NewApplyCommand creates an apply command with the given config.
func NewApplyCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "apply",
		Aliases:     []string{cmdNamePush},
		Usage:       fmt.Sprintf("Apply staged %s changes to %s", cfg.ItemName, remoteName(cfg.ProviderLabel)),
		ArgsUsage:   argsUsageName,
		Description: applyHelp(cfg),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  flagYes,
				Usage: usageSkipConfirm,
			},
			&cli.BoolFlag{
				Name:  "ignore-conflicts",
				Usage: fmt.Sprintf("Apply even if %s was modified after staging", remoteName(cfg.ProviderLabel)),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, resolved, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			opts := applyOptions{
				ignoreConflicts: cmd.Bool("ignore-conflicts"),
			}
			if cmd.Args().Len() > 0 {
				opts.name = cmd.Args().First()
			}

			strategy, err := cfg.Factory(ctx)
			if err != nil {
				return err
			}

			prompter := &confirm.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
				Target: resolved.Target.String(),
			}

			r := &applyRunner{
				useCase: &stagingusecase.ApplyUseCase{
					Strategy:    strategy,
					Store:       store,
					StrategyFor: applyStrategyFor(ctx, cfg.StrategyForNamespace),
				},
				store:         store,
				parser:        cfg.ParserFactory(),
				providerLabel: cfg.ProviderLabel,
				confirmer:     prompter,
				skipConfirm:   cmd.Bool(flagYes),
				stdout:        cmd.Root().Writer,
				stderr:        cmd.Root().ErrWriter,
			}

			return r.runInteractive(ctx, opts)
		},
	}
}

// NewResetCommand creates a reset command with the given config.
func NewResetCommand(cfg CommandConfig) *cli.Command {
	return &cli.Command{
		Name:        "reset",
		Usage:       fmt.Sprintf("Unstage %s or restore to specific version", cfg.ItemName),
		ArgsUsage:   "[spec]",
		Description: resetHelp(cfg),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: fmt.Sprintf("Unstage all %ss", cfg.ItemName),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			resetAll := cmd.Bool("all")

			if !resetAll && cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: %s reset <spec> or %s reset --all", cfg.CommandPath, cfg.CommandPath)
			}

			opts := resetOptions{
				all:       resetAll,
				namespace: cfg.namespaceFor(ctx),
			}

			if !resetAll {
				opts.spec = cmd.Args().First()
			}

			parser := cfg.ParserFactory()

			// Check if a version spec is provided. If so, we need a fetcher strategy
			// to restore the value from the remote store.
			var hasVersion bool

			if !resetAll && opts.spec != "" {
				var err error

				_, hasVersion, err = parser.ParseSpec(opts.spec)
				if err != nil {
					return err
				}
			}

			store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
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

			r := &resetRunner{
				useCase: &stagingusecase.ResetUseCase{
					Parser:  parser,
					Fetcher: fetcher,
					Store:   store,
				},
				stdout: cmd.Root().Writer,
				stderr: cmd.Root().ErrWriter,
			}

			return r.run(ctx, opts)
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
		Description: deleteHelp(cfg, hasDeleteOptions),
		Flags:       flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("usage: %s delete <name>", cfg.CommandPath)
			}

			store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
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

			r := &deleteRunner{
				useCase: &stagingusecase.DeleteUseCase{
					Strategy: strategy,
					Store:    store,
				},
				stdout: cmd.Root().Writer,
				stderr: cmd.Root().ErrWriter,
			}

			return r.run(ctx, deleteOptions{
				name:           name,
				force:          force,
				recoveryWindow: recoveryWindow,
				namespace:      cfg.namespaceFor(ctx),
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
			return fmt.Errorf("usage: %s %s", cfg.CommandPath, usageMsg)
		}

		name := cmd.Args().First()
		args := cmd.Args().Slice()[1:]

		store, _, err := openScopedWorkingStore(ctx, cfg.ScopeResolver)
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
		r := &tagRunner{
			useCase: useCase,
			stdout:  stdout,
			stderr:  stderr,
		}

		return r.run(ctx, tagOptions{name: name, namespace: cfg.namespaceFor(ctx), tags: tags})
	}

	return &cli.Command{
		Name:        "tag",
		Usage:       fmt.Sprintf("Stage tags for a %s", cfg.ItemName),
		ArgsUsage:   "<name> <key>=<value>...",
		Description: tagHelp(cfg),
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
		r := &untagRunner{
			useCase: useCase,
			stdout:  stdout,
			stderr:  stderr,
		}

		return r.run(ctx, untagOptions{name: name, namespace: cfg.namespaceFor(ctx), keys: keys})
	}

	return &cli.Command{
		Name:        "untag",
		Usage:       fmt.Sprintf("Stage tag removal for a %s", cfg.ItemName),
		ArgsUsage:   "<name> <key>...",
		Description: untagHelp(cfg),
		Action:      tagAction(cfg, "untag <name> <key>", runner),
	}
}
