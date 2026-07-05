// Package apptest provides helpers for constructing the CLI app with a
// deterministic provider-detection result, so command tests do not depend on
// the ambient environment (which decides the top-level param/secret aliases).
package apptest

import (
	"github.com/urfave/cli/v3"

	commands "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// AWSApp builds the app as if AWS were the uniquely active provider for both
// services, so the top-level `param` / `secret` aliases resolve to AWS
// regardless of the test environment. Use it in tests that exercise the AWS
// commands via `suve param ...` / `suve secret ...`.
func AWSApp() *cli.Command {
	return commands.MakeAppWithDetect(detect.Result{
		Param:  provider.ProviderAWS,
		Secret: provider.ProviderAWS,
		Stage:  provider.ProviderAWS,
	})
}
