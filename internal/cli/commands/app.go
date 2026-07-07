// Package commands provides the command-line interface for suve.
package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	awscmd "github.com/mpyw/suve/internal/cli/commands/aws"
	"github.com/mpyw/suve/internal/cli/commands/azure"
	"github.com/mpyw/suve/internal/cli/commands/gcloud"
	"github.com/mpyw/suve/internal/cli/commands/param"
	"github.com/mpyw/suve/internal/cli/commands/secret"
	"github.com/mpyw/suve/internal/cli/commands/stage"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// Version is set by goreleaser via ldflags.
//
//nolint:gochecknoglobals // build-time variable set by ldflags
var Version = "dev"

const baseUsage = "Git-like CLI for AWS Parameter Store / Secrets Manager, " +
	"Google Cloud Secret Manager, and Azure Key Vault / App Configuration"

// MakeApp creates a new CLI application instance, resolving the flat
// `param` / `secret` aliases from the current environment.
func MakeApp() *cli.Command {
	return MakeAppWithDetect(detect.Resolve(detect.OSEnvironment()))
}

// MakeAppWithDetect builds the app with an explicit provider-detection result.
// It is the injectable seam behind MakeApp: production passes the env-resolved
// result, while tests (and any caller needing determinism) pass a fixed one.
func MakeAppWithDetect(det detect.Result) *cli.Command {
	// The explicit provider groups are always present and unambiguous.
	commands := []*cli.Command{
		awscmd.Command(),
		gcloud.Command(),
		azure.Command(),
	}

	// Flat aliases are prepended only when a service resolves to exactly one
	// active provider (see internal/provider/detect).
	var flat []*cli.Command
	if c := flatCommand(det.Param, provider.KindParam); c != nil {
		flat = append(flat, c)
	}

	if c := flatCommand(det.Secret, provider.KindSecret); c != nil {
		flat = append(flat, c)
	}

	// The top-level `stage` alias appears when exactly one staging-capable
	// provider is active — consistent with param/secret. It is always reachable
	// explicitly as `suve aws stage` / `suve gcloud stage`.
	if c := flatStageCommand(det.Stage); c != nil {
		flat = append(flat, c)
	}

	return &cli.Command{
		Name:        "suve",
		Usage:       baseUsage,
		Description: aliasDescription(det),
		Version:     Version,
		Flags:       []cli.Flag{debugFlag()},
		Before:      enableDebug(det),
		Commands:    append(flat, commands...),
		// EnableShellCompletion adds a hidden `completion` command (bash/zsh/fish/pwsh)
		// and the `--generate-shell-completion` mechanism the scripts rely on.
		EnableShellCompletion: true,
		// Surface the completion command in help so it is discoverable, rather than
		// leaving it hidden as urfave/cli does by default.
		ConfigureShellCompletionCommand: func(c *cli.Command) {
			c.Hidden = false
		},
		CommandNotFound: func(_ context.Context, cmd *cli.Command, command string) {
			_ = cli.ShowAppHelp(cmd)
			w := lo.CoalesceOrEmpty(cmd.Root().ErrWriter, cmd.Root().Writer)
			output.Println(w, "")
			output.Warning(w, "Command not found: %s", command)
		},
	}
}

// debugFlag defines the global --debug switch. It is a persistent flag (v3
// flags propagate to subcommands unless marked Local), so it works in any
// position: `suve --debug sm ls` and `suve sm ls --debug` are equivalent. The
// SUVE_DEBUG environment variable is an alternative source.
func debugFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:    "debug",
		Usage:   "Log cloud SDK requests/responses to stderr (metadata only, no secret values)",
		Sources: cli.EnvVars("SUVE_DEBUG"),
	}
}

// enableDebug builds the root Before hook: when --debug (or SUVE_DEBUG) is set
// it stores a debug.Config in the context that provider adapters read to turn on
// their SDK request logging, and logs a one-shot summary of the decisions suve
// already made before any API call (version, flat-alias resolution). Debug
// output goes to the root ErrWriter so it never contaminates piped STDOUT.
func enableDebug(det detect.Result) cli.BeforeFunc {
	return func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if !cmd.Bool("debug") {
			return ctx, nil
		}

		cfg := debug.Config{
			Enabled: true,
			Writer:  lo.CoalesceOrEmpty[io.Writer](cmd.Root().ErrWriter, os.Stderr),
		}
		cfg.Logf("cli: suve version=%s\n", cmd.Root().Version)
		cfg.Logf("cli: flat aliases: param=%s secret=%s stage=%s%s\n",
			aliasTarget(det.Param), aliasTarget(det.Secret), aliasTarget(det.Stage), fallbackNote(det))

		return debug.With(ctx, cfg), nil
	}
}

// aliasTarget renders a flat-alias provider for the debug summary, making the
// "no alias" case explicit instead of printing an empty string.
func aliasTarget(p provider.Provider) string {
	return groupName(lo.CoalesceOrEmpty(p, "(none)"))
}

// fallbackNote annotates the debug alias summary when AWS became active only
// through the ~/.aws/credentials fallback rather than an env signal.
func fallbackNote(det detect.Result) string {
	return lo.Ternary(
		det.AWSViaFallback,
		" (AWS via ~/.aws/credentials fallback)",
		"",
	)
}

// flatCommand builds the top-level alias command (named "param" or "secret") for
// the given uniquely-active provider, or nil when there is none. It reuses each
// provider's real implementation so the alias behaves exactly like the explicit
// form.
func flatCommand(p provider.Provider, kind provider.Kind) *cli.Command {
	switch kind {
	case provider.KindParam:
		switch p {
		case provider.ProviderAWS:
			return param.Command()
		case provider.ProviderAzure:
			return azure.FlatParamCommand("param")
		case provider.ProviderGoogleCloud:
			// Google Cloud has no parameter store; never a param alias.
			return nil
		}
	case provider.KindSecret:
		switch p {
		case provider.ProviderAWS:
			return secret.Command()
		case provider.ProviderGoogleCloud:
			return gcloud.FlatSecretCommand("secret")
		case provider.ProviderAzure:
			return azure.FlatSecretCommand("secret")
		}
	}

	return nil
}

// flatStageCommand builds the top-level `stage` alias for the uniquely-active
// staging provider, or nil when there is none. It reuses each provider's real
// staging implementation so the alias behaves exactly like the explicit form.
func flatStageCommand(p provider.Provider) *cli.Command {
	switch p {
	case provider.ProviderAWS:
		return stage.Command()
	case provider.ProviderGoogleCloud:
		return gcloud.FlatStageCommand("stage")
	case provider.ProviderAzure:
		return azure.FlatStageCommand("stage")
	}

	return nil
}

// aliasDescription explains which flat aliases are active (and why), so the
// environment-dependent top-level help is self-documenting.
func aliasDescription(det detect.Result) string {
	var lines []string
	if det.FlatParam() {
		lines = append(lines, fmt.Sprintf("  param  -> %s", groupName(det.Param)))
	}

	if det.FlatSecret() {
		lines = append(lines, fmt.Sprintf("  secret -> %s", groupName(det.Secret)))
	}

	if det.FlatStage() {
		lines = append(lines, fmt.Sprintf("  stage  -> %s", groupName(det.Stage)))
	}

	if len(lines) == 0 {
		return "No provider is uniquely active in this environment, so there are no " +
			"top-level 'param'/'secret'/'stage' aliases. Use an explicit group: " +
			"'suve aws', 'suve gcloud', or 'suve azure'."
	}

	via := lo.Ternary(
		det.AWSViaFallback,
		" (AWS via ~/.aws/credentials)",
		" (from environment)",
	)

	return "Active top-level aliases" + via + ":\n" + strings.Join(lines, "\n") +
		"\nThe explicit groups ('suve aws', 'suve gcloud', 'suve azure') are always available."
}

// groupName maps a provider to its command-group name for user-facing messages.
func groupName(p provider.Provider) string {
	switch p {
	case provider.ProviderAWS:
		return "aws"
	case provider.ProviderGoogleCloud:
		return "gcloud"
	case provider.ProviderAzure:
		return "azure"
	}

	return string(p)
}

// App is the main CLI application.
//
//nolint:gochecknoglobals // singleton CLI app instance
var App = MakeApp()
