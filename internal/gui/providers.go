//go:build production || dev

package gui

import (
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// =============================================================================
// Provider detection
// =============================================================================

// DetectResult mirrors internal/provider/detect.Result for the frontend: the
// uniquely-active provider per service (empty when 0 or 2+ are active) plus the
// full active sets. It drives the GUI's initial provider selection — no
// priority order; when ambiguous the user picks.
type DetectResult struct {
	Param  string `json:"param"`
	Secret string `json:"secret"`
	Stage  string `json:"stage"`

	ParamActive  []string `json:"paramActive"`
	SecretActive []string `json:"secretActive"`
	StageActive  []string `json:"stageActive"`
}

// DetectProviders resolves which providers are active in the current
// environment (env-only, no network calls), for the GUI's initial selection.
func (a *App) DetectProviders() *DetectResult {
	r := detect.Resolve(detect.OSEnvironment())

	return &DetectResult{
		Param:        string(r.Param),
		Secret:       string(r.Secret),
		Stage:        string(r.Stage),
		ParamActive:  providerStrings(r.ParamActive),
		SecretActive: providerStrings(r.SecretActive),
		StageActive:  providerStrings(r.StageActive),
	}
}

// InitialProviderFromEnv resolves the initial provider for a bare `suve --gui`
// (and the standalone Wails entry) from the environment: the uniquely-active
// provider across services, or "" when zero or two-plus are active (the
// frontend then shows the selector). Env-only; no network calls.
func InitialProviderFromEnv() provider.Provider {
	return uniqueActiveProvider(detect.Resolve(detect.OSEnvironment()))
}

// uniqueActiveProvider returns the sole provider active across the param and
// secret services, or "" when zero or two-or-more distinct providers are
// active. Mirrors the CLI's per-service "exactly one active" rule, applied to
// the union across services for the GUI's initial provider pick.
func uniqueActiveProvider(r detect.Result) provider.Provider {
	seen := make(map[provider.Provider]struct{})

	for _, p := range r.SecretActive {
		seen[p] = struct{}{}
	}

	for _, p := range r.ParamActive {
		seen[p] = struct{}{}
	}

	if len(seen) != 1 {
		return ""
	}

	for p := range seen {
		return p
	}

	return ""
}

func providerStrings(ps []provider.Provider) []string {
	return lo.Map(ps, func(p provider.Provider, _ int) string { return string(p) })
}

// =============================================================================
// Capabilities
// =============================================================================

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

// descriptionSupported reports whether the current provider persists a
// create/update description. It is the server-side backstop for the capability-
// gated Description input: AWS (Param + Secret) and Google Cloud (Secret) honor
// it; Azure App Configuration and Key Vault writers ignore it, so the binding
// drops any description a stale/forged frontend might send (defense in depth,
// #767). Mirrors ServiceCapability.HasDescription.
func (a *App) descriptionSupported() bool {
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
