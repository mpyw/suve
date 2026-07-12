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

// ServiceCapability is re-exported from the neutral internal/capability package
// so the Wails binding surface (and the generated TypeScript models) stay stable
// while the matrix itself lives untagged and is shared with the TUI. The alias
// resolves to the same JSON shape, so the generated bindings need no
// regeneration.
type ServiceCapability = capability.ServiceCapability

// ProviderCapability is re-exported from the neutral internal/capability package
// so the Wails binding surface stays stable while the matrix lives untagged and
// is shared with the TUI. The alias resolves to the same JSON shape.
type ProviderCapability = capability.ProviderCapability

// Capabilities returns the static capability descriptor for every provider,
// driving provider-selection and control-visibility in the frontend. Display
// names: AWS {Param, Secret}, Google Cloud {Secret}, Azure {App Configuration,
// Key Vault}. The data lives in internal/capability so the TUI shares it.
func (a *App) Capabilities() []ProviderCapability {
	return capability.All()
}
