package commands

import (
	"context"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/provider"
)

// LaunchMode describes a UI that the CLI launches from a flag: `suve tui` via
// --tui and `suve gui` via --gui. RegisterLaunchMode attaches the flag the same
// way for both, so the provider groups, the Azure service subgroups, and the
// launch scope stay in step.
type LaunchMode struct {
	// Flag is the launch flag name ("tui" or "gui").
	Flag string
	// RootUsage is the root flag's usage text.
	RootUsage string
	// GroupUsage is the usage text on each provider group and service subgroup.
	GroupUsage string
	// Bare launches from the root flag (`suve --<flag>`). It resolves the
	// provider from the environment itself.
	Bare func(ctx context.Context) (context.Context, error)
	// Launch launches from a provider group or service subgroup. scope carries
	// the provider and the scope flags given on that command (not yet hydrated
	// from the environment; see detect.HydrateScope). service is "param" or
	// "secret" for a service subgroup, or "" for a group.
	Launch func(ctx context.Context, scope provider.Scope, service string) (context.Context, error)
}

// RegisterLaunchMode registers the mode's flag on the root command, each
// provider group, and Azure's param/secret subgroups. It WRAPS (never
// replaces) each command's Before hook, so it chains with the hooks already
// installed (enableDebug on the root, and any mode registered earlier). The
// launch is skipped during shell completion: urfave/cli runs Before hooks
// before the completion handler, so honoring the flag there would launch the UI
// instead of completing (#749).
func RegisterLaunchMode(m LaunchMode) {
	App.Flags = append(App.Flags, &cli.BoolFlag{Name: m.Flag, Usage: m.RootUsage})
	wrapLaunchBefore(App, m.Flag, func(ctx context.Context, _ *cli.Command) (context.Context, error) {
		return m.Bare(ctx)
	})

	for _, group := range App.Commands {
		p := launchGroupProvider(group.Name)
		if p == "" {
			continue
		}

		attachLaunchFlag(m, group, p, "")

		if p != provider.ProviderAzure {
			continue
		}

		for _, sub := range group.Commands {
			// Key off the canonical subcommand name (kv/keyvault already
			// resolved to "secret", store/appconfig to "param").
			if svc := launchService(sub.Name); svc != "" {
				attachLaunchFlag(m, sub, p, svc)
			}
		}
	}
}

// attachLaunchFlag adds the mode's flag to a provider group or service
// subgroup and launches with that provider, seeding the scope from the
// command's scope flags.
func attachLaunchFlag(m LaunchMode, cmd *cli.Command, p provider.Provider, service string) {
	cmd.Flags = append(cmd.Flags, &cli.BoolFlag{Name: m.Flag, Usage: m.GroupUsage})
	wrapLaunchBefore(cmd, m.Flag, func(ctx context.Context, c *cli.Command) (context.Context, error) {
		return m.Launch(ctx, launchScope(c, p), service)
	})
}

// wrapLaunchBefore wraps cmd's Before hook so a set launch flag runs launch,
// and anything else (including a shell-completion run) falls through to the
// previous hook.
func wrapLaunchBefore(
	cmd *cli.Command,
	flag string,
	launch func(ctx context.Context, c *cli.Command) (context.Context, error),
) {
	inner := cmd.Before
	cmd.Before = func(ctx context.Context, c *cli.Command) (context.Context, error) {
		if !isShellCompletionInvocation() && c.Bool(flag) {
			return launch(ctx, c)
		}

		if inner != nil {
			return inner(ctx, c)
		}

		return ctx, nil
	}
}

// launchScope builds the launch scope for provider p from the command's scope
// flags: --project for Google Cloud, and --vault-name / --store-name /
// --namespace for Azure. AWS has no scope flag (the region comes from the
// ambient AWS config). Absent flags stay empty, so detect.HydrateScope fills
// them from the environment and a given flag wins over env.
func launchScope(cmd *cli.Command, p provider.Provider) provider.Scope {
	s := provider.Scope{Provider: p}

	switch p {
	case provider.ProviderGoogleCloud:
		s.ProjectID = cmd.String("project")
	case provider.ProviderAzure:
		s.VaultName = cmd.String("vault-name")
		s.StoreName = cmd.String("store-name")
		s.AppConfigNamespace = cmd.String("namespace")
	case provider.ProviderAWS:
		// The region comes from the ambient AWS config; no scope flag.
	}

	return s
}

// launchGroupProvider maps a top-level command name to its provider, or ""
// when the command is not a provider group (e.g. a flat alias). It is the
// inverse of groupName.
func launchGroupProvider(name string) provider.Provider {
	p, _ := lo.Find(
		provider.Providers(),
		func(p provider.Provider) bool { return groupName(p) == name },
	)

	return p
}

// launchService maps a provider subgroup's canonical name to the launch
// service ("param"/"secret"), or "" when the command is not a service subgroup.
func launchService(name string) string {
	switch provider.Kind(name) {
	case provider.KindParam, provider.KindSecret:
		return name
	default:
		return ""
	}
}
