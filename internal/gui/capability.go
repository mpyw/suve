//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
)

// Capabilities.

// ServiceCapability and ProviderCapability are re-exported from the neutral
// internal/capability package so the matrix lives untagged and is shared with the
// TUI. The aliases keep the marshaled JSON byte-identical, so the committed
// wailsjs bindings under the gui namespace stay runtime-correct. Regenerating
// would resolve each alias to its underlying type and relocate both into a
// capability namespace in the generated TypeScript, forcing an update of every
// frontend ref (gui.ProviderCapability / gui.ServiceCapability) for no runtime
// gain — so the bindings are intentionally kept under gui and not regenerated for
// this refactor. capability.ServiceCapability already carries HasDescription, so
// the #767 Description-input gating works through the alias.
type ServiceCapability = capability.ServiceCapability

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

// ProviderCapability — see ServiceCapability above.
type ProviderCapability = capability.ProviderCapability

// Capabilities returns the static capability descriptor for every provider,
// driving provider-selection and control-visibility in the frontend. Display
// names: AWS {Param, Secret}, Google Cloud {Secret}, Azure {App Configuration,
// Key Vault}. The data lives in internal/capability so the TUI shares it.
func (a *App) Capabilities() []ProviderCapability {
	return capability.All()
}
