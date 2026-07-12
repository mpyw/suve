//go:build production || dev

package gui

import (
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// Service keys and display labels reused across the capability descriptors.
const (
	serviceParam      = "param"
	serviceSecret     = "secret"
	displayNameSecret = "Secret"
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
	// TagsPerVersion is true when tags are scoped to a specific version rather
	// than the resource (Azure Key Vault only): each version has its own tags,
	// so the GUI shows them per version in the history and writes target the
	// latest version. Every other provider keeps tags at the resource level.
	TagsPerVersion bool `json:"tagsPerVersion"`
	// HasRestore is true when a soft-deleted item can be restored (AWS Secrets
	// Manager only).
	HasRestore bool `json:"hasRestore"`
	// HasStaging is true when the GUI's staging workflow applies to this
	// service (every provider service today); the frontend hides the staging
	// tab/banner/checkbox when false.
	HasStaging bool `json:"hasStaging"`
	// HasNamespaces is true when the service partitions keys by a namespace (the
	// Azure App Configuration label axis, #431). The frontend shows the namespace
	// column/badge and the create-form namespace field only when true.
	HasNamespaces bool `json:"hasNamespaces"`
	// HasForceDelete is true when an immediate (no-recovery-window) delete is
	// offered (AWS Secrets Manager only). The frontend hides the force-delete
	// checkbox otherwise.
	HasForceDelete bool `json:"hasForceDelete"`
	// HasRecoveryWindow is true when a soft delete schedules a recovery window
	// whose end date can be surfaced (AWS Secrets Manager only). Other providers
	// delete immediately or govern retention by policy, so no "recoverable until"
	// date is shown.
	HasRecoveryWindow bool `json:"hasRecoveryWindow"`
}

// ProviderCapability describes a provider and the services it offers.
type ProviderCapability struct {
	// Provider is the internal key ("aws" | "googlecloud" | "azure").
	Provider string `json:"provider"`
	// DisplayName is the provider label (e.g. "Google Cloud").
	DisplayName string `json:"displayName"`
	// ScopeFields lists the provider-level scope inputs the frontend must collect
	// (e.g. ["project"] for Google Cloud). Empty for AWS (ambient config) and for
	// Azure, whose per-service vault/store names are collected by the service's
	// own view.
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
				{
					Service: serviceParam, DisplayName: "Param",
					HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: false,
					HasStaging: true, HasForceDelete: false, HasRecoveryWindow: false,
				},
				{
					Service: serviceSecret, DisplayName: displayNameSecret,
					HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: true,
					HasStaging: true, HasForceDelete: true, HasRecoveryWindow: true,
				},
			},
		},
		{
			Provider:    string(provider.ProviderGoogleCloud),
			DisplayName: "Google Cloud",
			ScopeFields: []string{"project"},
			Services: []ServiceCapability{
				{
					Service: serviceSecret, DisplayName: displayNameSecret,
					HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: false,
					HasStaging: true, HasForceDelete: false, HasRecoveryWindow: false,
				},
			},
		},
		{
			Provider:    string(provider.ProviderAzure),
			DisplayName: "Azure",
			ScopeFields: []string{},
			Services: []ServiceCapability{
				// App Configuration is unversioned; tags are writable via
				// GET-merge-PUT (azappconfig/v2).
				{
					Service: serviceParam, DisplayName: "App Configuration",
					HasVersionHistory: false, HasVersionSpecifiers: false, HasTags: true, HasRestore: false,
					HasStaging: true, HasForceDelete: false, HasRecoveryWindow: false, HasNamespaces: true,
				},
				{
					Service: serviceSecret, DisplayName: "Key Vault",
					HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, TagsPerVersion: true, HasRestore: true,
					// Force-delete (purge) is unsupported: Key Vault retention is a vault
					// property (softDeleteRetentionInDays), not a per-delete choice, and
					// staged deletes can't carry it — so deletes are always soft (Restore
					// recovers them). HasForceDelete/HasRecoveryWindow both stay false.
					HasStaging: true, HasForceDelete: false, HasRecoveryWindow: false,
				},
			},
		},
	}
}
