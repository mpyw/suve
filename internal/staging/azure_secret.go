package staging

import (
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

// AzureSecretStrategy implements the staging strategies for Azure Key
// Vault secrets over a provider.Store. Key Vault specifics:
//
//   - Versions are opaque ids, parsed with version.AzureKeyVault (#ID, ~SHIFT); a
//     staged "edit" applies as a new version via Put.
//   - There are no force / recovery-window delete options (delete is a soft
//     delete), so HasDeleteOptions reports false.
//   - Tags are writable, so tag/untag staging is supported.
//   - Conflict detection uses the secret's last-modified timestamp, like AWS.
//
// The zero value (no store) is a parser-only strategy (ParseName/ParseSpec).
type AzureSecretStrategy struct {
	versionedStrategy[azureSecretHooks]
}

// NewAzureSecretStrategy creates an Azure Key Vault staging strategy
// over the given provider store. A nil store is allowed for parser-only use.
func NewAzureSecretStrategy(store provider.Store) *AzureSecretStrategy {
	return &AzureSecretStrategy{versionedStrategy[azureSecretHooks]{store: store}}
}

// AzureSecretParserFactory yields a parser-only strategy.
func AzureSecretParserFactory() Parser {
	return NewAzureSecretStrategy(nil)
}

// azureSecretHooks supplies the Key Vault specifics.
type azureSecretHooks struct {
	versionedSecretHooks
}

func (azureSecretHooks) traits() versionedTraits {
	return versionedTraits{
		service:        ServiceSecret,
		serviceName:    "Key Vault",
		itemName:       secretServiceItemName,
		tagsFetchError: "failed to get secret",
	}
}

func (azureSecretHooks) parse(input string) (name, suffix string, err error) {
	return version.AzureKeyVault.Split(input)
}
