//go:build production || dev

package gui

import (
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

func providerStrings(ps []provider.Provider) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, string(p))
	}

	return out
}

// =============================================================================
// Capabilities
// =============================================================================

// ServiceCapability describes one provider service (param or secret) so the
// frontend can render the right label and hide unsupported controls.
type ServiceCapability struct {
	// Service is the internal key ("param" or "secret").
	Service string `json:"service"`
	// DisplayName is the label shown in the UI (e.g. "Key Vault").
	DisplayName string `json:"displayName"`
	// HasVersionHistory is true when `log`/history is supported (false for the
	// unversioned Azure App Configuration).
	HasVersionHistory bool `json:"hasVersionHistory"`
	// HasVersionSpecifiers is true when #VERSION/~SHIFT specifiers apply.
	HasVersionSpecifiers bool `json:"hasVersionSpecifiers"`
	// HasTags is true when tag/label read+write is supported (false for Azure
	// App Configuration).
	HasTags bool `json:"hasTags"`
	// HasRestore is true when a soft-deleted item can be restored (AWS Secrets
	// Manager only).
	HasRestore bool `json:"hasRestore"`
}

// ProviderCapability describes a provider and the services it offers.
type ProviderCapability struct {
	// Provider is the internal key ("aws" | "googlecloud" | "azure").
	Provider string `json:"provider"`
	// DisplayName is the provider label (e.g. "Google Cloud").
	DisplayName string `json:"displayName"`
	// ScopeFields lists the scope inputs the frontend must collect for this
	// provider (e.g. ["project"] for Google Cloud; ["subscription",
	// "resourceGroup"] for Azure — per-service vault/store are added by the
	// service's own view). Empty for AWS (ambient config).
	ScopeFields []string `json:"scopeFields"`
	// Services are the param/secret services this provider offers, in stable
	// display order.
	Services []ServiceCapability `json:"services"`
}

// Capabilities returns the static capability descriptor for every provider,
// driving provider-selection and control-visibility in the frontend. Display
// names: AWS {Param, Secret}, Google Cloud {Secret}, Azure {App Configuration,
// Key Vault}.
func (a *App) Capabilities() []ProviderCapability {
	return []ProviderCapability{
		{
			Provider:    string(provider.ProviderAWS),
			DisplayName: "AWS",
			ScopeFields: []string{},
			Services: []ServiceCapability{
				{Service: "param", DisplayName: "Param", HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: false},
				{Service: "secret", DisplayName: "Secret", HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: true},
			},
		},
		{
			Provider:    string(provider.ProviderGoogleCloud),
			DisplayName: "Google Cloud",
			ScopeFields: []string{"project"},
			Services: []ServiceCapability{
				{Service: "secret", DisplayName: "Secret", HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: false},
			},
		},
		{
			Provider:    string(provider.ProviderAzure),
			DisplayName: "Azure",
			ScopeFields: []string{"subscription", "resourceGroup"},
			Services: []ServiceCapability{
				// App Configuration is unversioned and cannot write tags.
				{Service: "param", DisplayName: "App Configuration", HasVersionHistory: false, HasVersionSpecifiers: false, HasTags: false, HasRestore: false},
				{Service: "secret", DisplayName: "Key Vault", HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: false},
			},
		},
	}
}
