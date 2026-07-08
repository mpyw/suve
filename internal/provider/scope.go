package provider

import "fmt"

// Scope identifies a provider-specific namespace for staging state. The set of
// meaningful fields depends on Provider:
//
//   - AWS: AccountID + Region (shared for param and secret).
//   - GoogleCloud: ProjectID (Secret Manager only).
//   - Azure: VaultName (Key Vault, secret) or StoreName (App Configuration,
//     param) — each a globally-unique name that fully identifies the resource,
//     so no subscription/resource-group is needed.
//
// Scope is used both to select a provider factory (Provider field) and to key
// on-disk staging storage (see Key).
type Scope struct {
	// Provider selects which backend the scope belongs to.
	Provider Provider `json:"provider"`

	// AccountID is the AWS account id (AWS).
	AccountID string `json:"accountId,omitempty"`
	// Region is the AWS region (AWS).
	Region string `json:"region,omitempty"`

	// ProjectID is the Google Cloud project id (GoogleCloud).
	ProjectID string `json:"projectId,omitempty"`

	// VaultName is the Azure Key Vault name (Azure, secret).
	VaultName string `json:"vaultName,omitempty"`
	// StoreName is the Azure App Configuration store name (Azure, param).
	StoreName string `json:"storeName,omitempty"`
	// AppConfigNamespace is the Azure App Configuration namespace — the axis
	// Azure calls a "label" — selected for the store (Azure, param). Empty is
	// the null (default) namespace. A single resolved namespace only; the
	// filter grammar (`*`/`,`) never reaches staging (see #381).
	AppConfigNamespace string `json:"appConfigNamespace,omitempty"`
}

// Key returns a stable, filesystem-safe key identifying the scope. It is used
// to key on-disk staging storage (e.g. ~/.suve/staging/{Key()}/param.json).
func (s Scope) Key() string {
	switch s.Provider {
	case ProviderAWS:
		return fmt.Sprintf("aws/%s/%s", s.AccountID, s.Region)
	case ProviderGoogleCloud:
		return fmt.Sprintf("googlecloud/%s", s.ProjectID)
	case ProviderAzure:
		// Key Vault and App Configuration names are globally unique DNS labels
		// (*.vault.azure.net / *.azconfig.io), so the name alone identifies the
		// resource — no subscription/resource-group needed.
		if s.VaultName != "" {
			return fmt.Sprintf("azure/keyvault/%s", s.VaultName)
		}

		// The null (default) namespace keeps the original backward-compatible
		// path; a named namespace makes staging state per-(store, namespace).
		if s.AppConfigNamespace != "" {
			return fmt.Sprintf("azure/appconfig/%s/%s", s.StoreName, s.AppConfigNamespace)
		}

		return fmt.Sprintf("azure/appconfig/%s", s.StoreName)
	default:
		return ""
	}
}

// SupportsService reports whether the scope's provider offers the given store
// kind. AWS supports both param and secret; GoogleCloud supports secret only;
// Azure supports secret (Key Vault) or param (App Configuration) depending on
// which of VaultName/StoreName is set.
func (s Scope) SupportsService(kind Kind) bool {
	switch s.Provider {
	case ProviderAWS:
		return kind == KindParam || kind == KindSecret
	case ProviderGoogleCloud:
		return kind == KindSecret
	case ProviderAzure:
		if s.VaultName != "" {
			return kind == KindSecret
		}

		return kind == KindParam
	default:
		return false
	}
}

// SupportedKinds returns the store kinds the scope supports, in a stable order
// (KindParam, then KindSecret). This is the registry-driven iteration source
// that replaces hardcoded {param, secret} loops.
func (s Scope) SupportedKinds() []Kind {
	var kinds []Kind

	for _, k := range []Kind{KindParam, KindSecret} {
		if s.SupportsService(k) {
			kinds = append(kinds, k)
		}
	}

	return kinds
}

// AWSScope creates a Scope for AWS from an account id and region.
func AWSScope(accountID, region string) Scope {
	return Scope{
		Provider:  ProviderAWS,
		AccountID: accountID,
		Region:    region,
	}
}

// GoogleCloudScope creates a Scope for Google Cloud from a project id.
func GoogleCloudScope(projectID string) Scope {
	return Scope{
		Provider:  ProviderGoogleCloud,
		ProjectID: projectID,
	}
}

// AzureKeyVaultScope creates a Scope for an Azure Key Vault (secret store).
// The vault name is globally unique, so it fully identifies the resource.
func AzureKeyVaultScope(vaultName string) Scope {
	return Scope{
		Provider:  ProviderAzure,
		VaultName: vaultName,
	}
}

// AzureAppConfigScope creates a Scope for an Azure App Configuration store
// (param store). The store name is globally unique, so it fully identifies the
// resource.
func AzureAppConfigScope(storeName string) Scope {
	return Scope{
		Provider:  ProviderAzure,
		StoreName: storeName,
	}
}
