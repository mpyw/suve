package commands

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
	"github.com/mpyw/suve/internal/tui"
)

// tuiFlagName is the name of the --tui launch flag, attached to the root, each
// provider group, and the service and stage subgroups (see RegisterLaunchMode).
const tuiFlagName = "tui"

// RegisterTUIFlag registers the --tui launch flag through RegisterLaunchMode
// (the registration `suve --gui` shares). On --tui the hook resolves the
// provider + scope and short-circuits into the TUI, so it is registered here in
// the untagged commands package (unlike --gui there is no tagged stub split).
// Call it from main after registerGUIFlag so the --tui wrapper sits outermost.
func RegisterTUIFlag() {
	RegisterLaunchMode(LaunchMode{
		Flag:       tuiFlagName,
		RootUsage:  "Launch TUI mode (requires exactly one active provider; use 'suve <provider> --tui' otherwise)",
		GroupUsage: "Launch TUI mode for this provider",
		Bare:       launchTUIBare,
		Launch:     launchTUI,
	})
}

// RegisterTUIDescription rewrites the root usage to advertise the TUI. It must
// run BEFORE registerGUIDescription so the two compose to "CLI/GUI/TUI" in the
// GUI build: this turns "CLI" into "CLI/TUI", then the GUI rewrite turns the
// leading "CLI" into "CLI/GUI", yielding "CLI/GUI/TUI". In the default build the
// GUI rewrite is a no-op, leaving "CLI/TUI".
func RegisterTUIDescription() {
	App.Usage = strings.Replace(App.Usage, "CLI", "CLI/TUI", 1)
}

// launchTUIBare handles `suve --tui`: it launches only when exactly one provider
// is active (detect.Result.UniqueProvider, the rule `suve --gui` shares), and
// otherwise returns a friendly error naming the candidates and the explicit
// forms.
func launchTUIBare(ctx context.Context) (context.Context, error) {
	det := detect.Resolve(detect.OSEnvironment())

	p, err := uniqueTUIProvider(det)
	if err != nil {
		return ctx, err
	}

	// No scope flags on the bare form; detect.HydrateScope fills resource fields
	// from the environment. No specific service (group-level launch).
	return launchTUI(ctx, provider.Scope{Provider: p}, "")
}

// launchTUI performs the TTY guard, hydrates and validates the scope, runs the
// TUI, and exits — the short-circuit shared by every --tui hook.
func launchTUI(ctx context.Context, scope provider.Scope, service string) (context.Context, error) {
	if err := requireTUITTY(); err != nil {
		return ctx, err
	}

	scope, err := detect.HydrateScope(detect.OSEnvironment(), scope)
	if err != nil {
		return ctx, err
	}

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

// uniqueTUIProvider returns the sole active provider (detect.Result.UniqueProvider),
// or an error listing the candidates (or, when none is active, all providers).
func uniqueTUIProvider(det detect.Result) (provider.Provider, error) {
	if p := det.UniqueProvider(); p != "" {
		return p, nil
	}

	active := det.ActiveProviders()
	if len(active) == 0 {
		return "", errors.New(noActiveTUIProviderMessage())
	}

	return "", errors.New(ambiguousTUIProviderMessage(active))
}

// ambiguousTUIProviderMessage renders the "pick a provider" error for 2+ active
// providers, naming the candidates and the explicit `suve <group> --tui` forms.
func ambiguousTUIProviderMessage(active []provider.Provider) string {
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

// noActiveTUIProviderMessage renders the error for a bare `suve --tui` when no
// provider is active, listing every explicit form.
func noActiveTUIProviderMessage() string {
	cmds := []string{
		"  suve aws --tui",
		"  suve gcloud --tui",
		"  suve azure --tui",
	}

	return "no provider is active in this environment.\n" +
		"Launch the TUI with an explicit provider:\n\n" +
		strings.Join(cmds, "\n")
}
