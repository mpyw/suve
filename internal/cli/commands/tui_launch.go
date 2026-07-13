package commands

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
	"github.com/mpyw/suve/internal/tui"
)

// tuiFlagName is the name of the --tui launch flag, attached to the root, each
// provider group, and Azure's service subgroups.
const tuiFlagName = "tui"

// RegisterTUIFlag registers the --tui launch flag on the root command, each
// provider group, and Azure's param/secret subgroups. It WRAPS (never replaces)
// each command's Before hook so it chains with the hooks already installed:
// enableDebug on the root and, in the GUI-embedded build, the --gui wrapper
// applied by registerGUIFlag. On --tui the hook resolves the provider + scope
// and short-circuits into the TUI, so it is registered here in the untagged
// commands package (unlike --gui there is no tagged stub split). Call it from
// main after registerGUIFlag so the --tui wrapper sits outermost.
func RegisterTUIFlag() {
	App.Flags = append(App.Flags, &cli.BoolFlag{
		Name:  tuiFlagName,
		Usage: "Launch TUI mode (requires exactly one active provider; use 'suve <provider> --tui' otherwise)",
	})

	inner := App.Before
	App.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		// Skip the launch during shell completion: urfave/cli runs Before hooks
		// before the completion handler, so honoring --tui here would enter the TUI
		// (which errors on the non-TTY completion pipe) instead of completing (#749).
		if !isShellCompletionInvocation() && cmd.Bool(tuiFlagName) {
			return launchTUIBare(ctx)
		}

		if inner != nil {
			return inner(ctx, cmd)
		}

		return ctx, nil
	}

	for _, group := range App.Commands {
		p := tuiGroupProvider(group.Name)
		if p == "" {
			continue
		}

		attachTUIFlag(group, p, "")

		if p == provider.ProviderAzure {
			for _, sub := range group.Commands {
				// Key off the canonical subcommand name (kv/keyvault already resolved
				// to "secret", store/appconfig to "param"), matching attachGUIFlag.
				if svc := tuiService(sub.Name); svc != "" {
					attachTUIFlag(sub, p, svc)
				}
			}
		}
	}
}

// RegisterTUIDescription rewrites the root usage to advertise the TUI. It must
// run BEFORE registerGUIDescription so the two compose to "CLI/GUI/TUI" in the
// GUI build: this turns "CLI" into "CLI/TUI", then the GUI rewrite turns the
// leading "CLI" into "CLI/GUI", yielding "CLI/GUI/TUI". In the default build the
// GUI rewrite is a no-op, leaving "CLI/TUI".
func RegisterTUIDescription() {
	App.Usage = strings.Replace(App.Usage, "CLI", "CLI/TUI", 1)
}

// attachTUIFlag adds a --tui flag to a command (a provider group or one of its
// service subgroups) and wraps its Before hook so --tui launches the TUI with
// that provider, seeding the initial scope from the command's scope flags.
// service ("param"/"secret", or "" for a group) preselects the initial tab.
func attachTUIFlag(cmd *cli.Command, p provider.Provider, service string) {
	cmd.Flags = append(cmd.Flags, &cli.BoolFlag{
		Name:  tuiFlagName,
		Usage: "Launch TUI mode for this provider",
	})

	inner := cmd.Before
	cmd.Before = func(ctx context.Context, c *cli.Command) (context.Context, error) {
		// Skip the launch during shell completion (see the root wrapper, #749).
		if !isShellCompletionInvocation() && c.Bool(tuiFlagName) {
			return launchTUI(ctx, tuiScope(c, p), service)
		}

		if inner != nil {
			return inner(ctx, c)
		}

		return ctx, nil
	}
}

// launchTUIBare handles `suve --tui`: it launches only when exactly one provider
// is active across the union of all service axes (param + secret + stage), and
// otherwise returns a friendly error naming the candidates and the explicit
// forms.
func launchTUIBare(ctx context.Context) (context.Context, error) {
	det := detect.Resolve(detect.OSEnvironment())

	p, err := uniqueTUIProvider(det)
	if err != nil {
		return ctx, err
	}

	// No scope flags on the bare form; hydrateTUIScope fills resource fields from
	// the environment. No specific service (group-level launch).
	return launchTUI(ctx, provider.Scope{Provider: p}, "")
}

// launchTUI performs the TTY guard, hydrates and validates the scope, runs the
// TUI, and exits — the short-circuit shared by every --tui hook.
func launchTUI(ctx context.Context, scope provider.Scope, service string) (context.Context, error) {
	if err := requireTUITTY(); err != nil {
		return ctx, err
	}

	scope = hydrateTUIScope(scope)

	if err := validateTUIScope(scope); err != nil {
		return ctx, err
	}

	if err := tui.Run(ctx, scope, service); err != nil {
		return ctx, err
	}

	os.Exit(0)

	return ctx, nil
}

// requireTUITTY rejects a non-interactive launch: the TUI takes over the screen
// and reads keys, so both stdin and stdout must be terminals (same spirit as
// the $EDITOR TTY gate).
func requireTUITTY() error {
	if !terminal.IsTerminalReader(os.Stdin) || !terminal.IsTerminalWriter(os.Stdout) {
		return errors.New("the TUI requires an interactive terminal (a TTY on both stdin and stdout)")
	}

	return nil
}

// tuiScope builds the launch scope for provider p from the command's scope
// flags (--project for Google Cloud; --vault-name / --store-name / --namespace
// for Azure). Absent flags stay empty and are hydrated from the environment by
// hydrateTUIScope. It mirrors the GUI's guiScope.
func tuiScope(cmd *cli.Command, p provider.Provider) provider.Scope {
	s := provider.Scope{Provider: p}

	switch p {
	case provider.ProviderGoogleCloud:
		s.ProjectID = cmd.String("project")
	case provider.ProviderAzure:
		s.VaultName = cmd.String("vault-name")
		s.StoreName = cmd.String("store-name")
		s.AppConfigNamespace = cmd.String("namespace")
	case provider.ProviderAWS:
		// region comes from the ambient AWS config; no scope flag.
	}

	return s
}

// hydrateTUIScope fills empty resource fields on a launch scope from the
// environment (flag wins over env), mirroring the GUI's hydrateScope.
func hydrateTUIScope(s provider.Scope) provider.Scope {
	switch s.Provider {
	case provider.ProviderGoogleCloud:
		if s.ProjectID == "" {
			s.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
		}
	case provider.ProviderAzure:
		if s.VaultName == "" {
			s.VaultName = os.Getenv("AZURE_KEYVAULT_NAME")
		}

		if s.StoreName == "" {
			s.StoreName = os.Getenv("AZURE_APPCONFIG_NAME")
		}

		if s.AppConfigNamespace == "" {
			s.AppConfigNamespace = os.Getenv("AZURE_APPCONFIG_NAMESPACE")
		}
	case provider.ProviderAWS:
		// region comes from the ambient AWS config; nothing to hydrate.
	}

	return s
}

// validateTUIScope rejects a launch scope that cannot resolve any service, with
// guidance naming the flags/env to set.
func validateTUIScope(s provider.Scope) error {
	switch s.Provider {
	case provider.ProviderGoogleCloud:
		if s.ProjectID == "" {
			return errors.New("no Google Cloud project: set --project or the GOOGLE_CLOUD_PROJECT environment variable")
		}
	case provider.ProviderAzure:
		if s.VaultName == "" && s.StoreName == "" {
			return errors.New(
				"no Azure Key Vault or App Configuration store: set --vault-name / --store-name " +
					"or the AZURE_KEYVAULT_NAME / AZURE_APPCONFIG_NAME environment variable",
			)
		}
	case provider.ProviderAWS:
		// AWS resolves its region from the ambient config; nothing to validate.
	}

	return nil
}

// uniqueTUIProvider returns the sole provider active across the union of the
// param, secret, and stage service axes, or an error listing the candidates
// (or, when none is active, all providers).
func uniqueTUIProvider(det detect.Result) (provider.Provider, error) {
	active := activeTUIProviders(det)

	switch len(active) {
	case 1:
		return active[0], nil
	case 0:
		return "", errors.New(noActiveProviderMessage())
	default:
		return "", errors.New(ambiguousProviderMessage(active))
	}
}

// activeTUIProviders lists every provider active in any service axis, in stable
// order (AWS, Google Cloud, Azure).
func activeTUIProviders(det detect.Result) []provider.Provider {
	present := make(map[provider.Provider]bool)

	for _, set := range [][]provider.Provider{det.ParamActive, det.SecretActive, det.StageActive} {
		for _, p := range set {
			present[p] = true
		}
	}

	var out []provider.Provider

	for _, p := range []provider.Provider{provider.ProviderAWS, provider.ProviderGoogleCloud, provider.ProviderAzure} {
		if present[p] {
			out = append(out, p)
		}
	}

	return out
}

// ambiguousProviderMessage renders the "pick a provider" error for 2+ active
// providers, naming the candidates and the explicit `suve <group> --tui` forms.
func ambiguousProviderMessage(active []provider.Provider) string {
	names := make([]string, len(active))
	cmds := make([]string, len(active))

	for i, p := range active {
		names[i] = string(p)
		cmds[i] = "  suve " + groupName(p) + " --tui"
	}

	return "multiple providers are active (" + strings.Join(names, ", ") + ").\n" +
		"Launch the TUI with an explicit provider:\n\n" +
		strings.Join(cmds, "\n")
}

// noActiveProviderMessage renders the error for a bare `suve --tui` when no
// provider is active, listing every explicit form.
func noActiveProviderMessage() string {
	cmds := []string{
		"  suve aws --tui",
		"  suve gcloud --tui",
		"  suve azure --tui",
	}

	return "no provider is active in this environment.\n" +
		"Launch the TUI with an explicit provider:\n\n" +
		strings.Join(cmds, "\n")
}

// tuiGroupProvider maps a top-level command group name to its provider, or ""
// when the command is not a provider group (e.g. a flat alias).
func tuiGroupProvider(name string) provider.Provider {
	switch name {
	case "aws":
		return provider.ProviderAWS
	case "gcloud":
		return provider.ProviderGoogleCloud
	case "azure":
		return provider.ProviderAzure
	default:
		return ""
	}
}

// tuiService maps a provider subgroup's canonical name to the launch service
// ("param"/"secret"), or "" when the command is not a service subgroup.
func tuiService(name string) string {
	switch name {
	case "param", "secret":
		return name
	default:
		return ""
	}
}
