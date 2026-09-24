//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
)

// Capabilities.

// serviceCapability returns the capability of one service kind of the current
// provider. The bindings gate provider-specific
// behavior on it, as server-side backstops for the capability-gated controls
// (defense in depth, #767): a stale or forged frontend call cannot reach a
// feature the service does not have. An unknown service or provider has no
// capabilities.
//
//declscope:package // the param and secret bindings gate description, value type and recovery window on it
func (a *App) serviceCapability(kind provider.Kind) capability.ServiceCapability {
	sc, _ := capability.Service(a.currentScope().Provider, string(kind))

	return sc
}

// Capabilities returns the static capability descriptor for every provider,
// driving provider-selection and control-visibility in the frontend. Display
// names: AWS {Param, Secret}, Google Cloud {Secret}, Azure {App Configuration,
// Key Vault}. The data lives in internal/capability so the TUI shares it.
func (a *App) Capabilities() []capability.ProviderCapability {
	return capability.All()
}
