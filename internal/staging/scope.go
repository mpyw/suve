package staging

import "fmt"

// Provider represents a cloud provider.
type Provider string

const (
	// ProviderAWS represents Amazon Web Services.
	ProviderAWS Provider = "aws"
	// ProviderGoogleCloud represents Google Cloud Platform.
	ProviderGoogleCloud Provider = "googlecloud"
	// ProviderAzure represents Microsoft Azure.
	ProviderAzure Provider = "azure"
)

// Scope represents the staging scope.
// Required fields vary by provider:
//   - AWS: AccountID + Region (shared for param/secret)
//   - GoogleCloud: ProjectID (secret only)
//   - Azure: SubscriptionID + ResourceGroup + VaultName or StoreName
type Scope struct {
	Provider Provider `json:"provider"`

	// AWS fields
	AccountID string `json:"accountId,omitempty"`
	Region    string `json:"region,omitempty"`

	// GoogleCloud fields
	ProjectID string `json:"projectId,omitempty"`

	// Azure fields
	SubscriptionID string `json:"subscriptionId,omitempty"`
	ResourceGroup  string `json:"resourceGroup,omitempty"`
	VaultName      string `json:"vaultName,omitempty"` // KeyVault (secret)
	StoreName      string `json:"storeName,omitempty"` // AppConfig (param)
}

// Key returns a unique key for file paths.
func (s Scope) Key() string {
	switch s.Provider {
	case ProviderAWS:
		return fmt.Sprintf("aws/%s/%s", s.AccountID, s.Region)
	case ProviderGoogleCloud:
		return fmt.Sprintf("googlecloud/%s", s.ProjectID)
	case ProviderAzure:
		if s.VaultName != "" {
			return fmt.Sprintf("azure/%s/%s/keyvault/%s", s.SubscriptionID, s.ResourceGroup, s.VaultName)
		}

		return fmt.Sprintf("azure/%s/%s/appconfig/%s", s.SubscriptionID, s.ResourceGroup, s.StoreName)
	default:
		return ""
	}
}

// SupportsService returns true if the scope supports the given service type.
func (s Scope) SupportsService(svc Service) bool {
	switch s.Provider {
	case ProviderAWS:
		return true // supports both param and secret
	case ProviderGoogleCloud:
		return svc == ServiceSecret // Secret Manager only
	case ProviderAzure:
		if s.VaultName != "" {
			return svc == ServiceSecret
		}

		return svc == ServiceParam
	default:
		return false
	}
}

// AWSScope creates a Scope for AWS.
func AWSScope(accountID, region string) Scope {
	return Scope{
		Provider:  ProviderAWS,
		AccountID: accountID,
		Region:    region,
	}
}

// GoogleCloudScope creates a Scope for Google Cloud.
func GoogleCloudScope(projectID string) Scope {
	return Scope{
		Provider:  ProviderGoogleCloud,
		ProjectID: projectID,
	}
}

// AzureKeyVaultScope creates a Scope for Azure Key Vault.
func AzureKeyVaultScope(subscriptionID, resourceGroup, vaultName string) Scope {
	return Scope{
		Provider:       ProviderAzure,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		VaultName:      vaultName,
	}
}

// AzureAppConfigScope creates a Scope for Azure App Configuration.
func AzureAppConfigScope(subscriptionID, resourceGroup, storeName string) Scope {
	return Scope{
		Provider:       ProviderAzure,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		StoreName:      storeName,
	}
}
