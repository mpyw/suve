package staging

import (
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

// AzureKeyVaultSecretStrategy implements the staging strategies for Azure Key
// Vault secrets over a provider.Store. Key Vault specifics:
//
//   - Versions are opaque ids, parsed with version.KeyVault (#ID, ~SHIFT); a
//     staged "edit" applies as a new version via Put.
//   - There are no force / recovery-window delete options (delete is a soft
//     delete), so HasDeleteOptions reports false.
//   - Tags are writable, so tag/untag staging is supported.
//   - Conflict detection uses the secret's last-modified timestamp, like AWS.
//
// The zero value (no store) is a parser-only strategy (ParseName/ParseSpec).
type AzureKeyVaultSecretStrategy struct {
	versionedStrategy[azureKeyVaultSecretHooks]
}

// NewAzureKeyVaultSecretStrategy creates an Azure Key Vault staging strategy
// over the given provider store. A nil store is allowed for parser-only use.
func NewAzureKeyVaultSecretStrategy(store provider.Store) *AzureKeyVaultSecretStrategy {
	return &AzureKeyVaultSecretStrategy{versionedStrategy[azureKeyVaultSecretHooks]{store: store}}
}

// AzureKeyVaultSecretParserFactory yields a parser-only strategy.
func AzureKeyVaultSecretParserFactory() Parser {
	return NewAzureKeyVaultSecretStrategy(nil)
}

// azureKeyVaultSecretHooks supplies the Key Vault specifics.
type azureKeyVaultSecretHooks struct {
	versionedSecretHooks
}

func (azureKeyVaultSecretHooks) traits() versionedTraits {
	return versionedTraits{
		service:        ServiceSecret,
		serviceName:    "Key Vault",
		itemName:       itemNameSecret,
		tagsFetchError: "failed to get secret",
	}
}

func (azureKeyVaultSecretHooks) parse(input string) (name, suffix string, err error) {
	return version.KeyVault.Split(input)
}
