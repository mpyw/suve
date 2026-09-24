//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
)

// Capabilities.

// hasDescriptionCapability reports whether the current provider persists a
// create/update description. It is the server-side backstop for the capability-
// gated Description input: AWS (Param + Secret) and Google Cloud (Secret) honor
// it; Azure App Configuration and Key Vault writers ignore it, so the binding
// drops any description a stale/forged frontend might send (defense in depth,
// #767). Mirrors ServiceCapability.HasDescription.
//
//declscope:package // the param and secret write bindings drop an unsupported description
func (a *App) hasDescriptionCapability() bool {
	return a.currentScope().Provider != provider.ProviderAzure
}

// Capabilities returns the static capability descriptor for every provider,
// driving provider-selection and control-visibility in the frontend. Display
// names: AWS {Param, Secret}, Google Cloud {Secret}, Azure {App Configuration,
// Key Vault}. The data lives in internal/capability so the TUI shares it.
func (a *App) Capabilities() []capability.ProviderCapability {
	return capability.All()
}
