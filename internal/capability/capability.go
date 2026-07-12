// Package capability holds the provider×service capability matrix — the static,
// provider-neutral descriptor that drives conditional UI in every frontend. It
// carries no build tag and no SDK or Wails dependency, so the GUI (build-tagged)
// and the TUI (default build) consume one source of truth instead of duplicating
// the matrix.
package capability

import (
	"github.com/mpyw/suve/internal/provider"
)

// Service keys and display labels reused across the capability descriptors.
const (
	serviceParam      = "param"
	serviceSecret     = "secret"
	displayNameSecret = "Secret"
)

// ServiceCapability describes one provider service (param or secret) so a
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
	// so the frontend shows them per version in the history and writes target the
	// latest version. Every other provider keeps tags at the resource level.
	TagsPerVersion bool `json:"tagsPerVersion"`
	// HasRestore is true when a soft-deleted item can be restored (AWS Secrets
	// Manager only).
	HasRestore bool `json:"hasRestore"`
	// HasStaging is true when the frontend's staging workflow applies to this
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
	// HasDescription is true when a write carries a free-text description (AWS
	// Parameter Store + Secrets Manager only). The gcloud, Azure Key Vault, and
	// Azure App Configuration writers ignore a description, so the frontend hides
	// the description field for them (GUI parity: only AWS forms offer it).
	HasDescription bool `json:"hasDescription"`
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

// All returns the static capability descriptor for every provider, driving
// provider-selection and control-visibility in the frontends. Display names:
// AWS {Param, Secret}, Google Cloud {Secret}, Azure {App Configuration,
// Key Vault}.
func All() []ProviderCapability {
	return []ProviderCapability{
		{
			Provider:    string(provider.ProviderAWS),
			DisplayName: "AWS",
			ScopeFields: []string{},
			Services: []ServiceCapability{
				{
					Service: serviceParam, DisplayName: "Param",
					HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: false,
					HasStaging: true, HasForceDelete: false, HasRecoveryWindow: false, HasDescription: true,
				},
				{
					Service: serviceSecret, DisplayName: displayNameSecret,
					HasVersionHistory: true, HasVersionSpecifiers: true, HasTags: true, HasRestore: true,
					HasStaging: true, HasForceDelete: true, HasRecoveryWindow: true, HasDescription: true,
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
