// Package commands provides the command-line interface for suve.
package commands

import (
	"context"
	"fmt"
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
		Commands:    append(flat, commands...),
		CommandNotFound: func(_ context.Context, cmd *cli.Command, command string) {
			_ = cli.ShowAppHelp(cmd)
			w := lo.CoalesceOrEmpty(cmd.Root().ErrWriter, cmd.Root().Writer)
			output.Println(w, "")
			output.Warning(w, "Command not found: %s", command)
		},
	}
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
		// Azure is not yet staging-capable.
		return nil
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

	via := " (from environment)"
	if det.AWSViaFallback {
		via = " (AWS via ~/.aws/credentials)"
	}

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
