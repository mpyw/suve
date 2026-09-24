//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/provider"
)

// parseSpec splits a version spec into its name and version suffix with the
// grammar of the active provider's service, looked up in the shared staging
// binding (the table the CLI and TUI use). The use cases hand the suffix to
// provider.Reader.Resolve, which re-parses it with the same grammar.
//
// A provider-specific grammar matters: the AWS grammar would split an Azure App
// Configuration key that legally contains '#' or '~' into a bogus
// name+version, and Google Cloud and Key Vault reject a ':LABEL' spec that AWS
// Secrets Manager accepts.
//
//declscope:package // shared with the param and secret namespaces
func (a *App) parseSpec(kind provider.Kind, specStr string) (name, suffix string, err error) {
	b, err := a.stagingBinding(a.currentScope(), string(kind))
	if err != nil {
		return "", "", err
	}

	return b.SplitSpec(specStr)
}
