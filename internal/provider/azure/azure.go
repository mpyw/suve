// Package azure wires the two Azure adapters into a provider.Factory /
// provider.Registry:
//
//   - KindSecret -> Azure Key Vault (secrets), addressed by VaultName.
//   - KindParam  -> Azure App Configuration (params), addressed by StoreName.
//
// Both clients authenticate via azidentity.NewDefaultAzureCredential (the
// DefaultAzureCredential chain: environment, workload identity, managed
// identity, Azure CLI, ...). The concrete Azure SDK clients are built here and
// handed to the keyvault / appconfig subpackages, which confine every Azure SDK
// type behind the provider seam.
package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/azure/keyvault"
)

// Factory builds Azure-backed provider.Store values for a scope + kind.
type Factory struct{}

// Compile-time assertion that Factory implements provider.Factory.
var _ provider.Factory = Factory{}

// Store builds a Store for the given scope and kind. KindSecret yields a Key
// Vault store (requires scope.VaultName); KindParam yields an App Configuration
// store (requires scope.StoreName). A missing required field yields a clear
// error; an unknown kind yields provider.ErrUnsupportedKind.
func (Factory) Store(ctx context.Context, scope provider.Scope, kind provider.Kind) (provider.Store, error) {
	switch kind {
	case provider.KindSecret:
		return keyVaultStore(ctx, scope)
	case provider.KindParam:
		return appConfigStore(ctx, scope)
	default:
		return nil, fmt.Errorf("%w: %s", provider.ErrUnsupportedKind, kind)
	}
}

// keyVaultStore builds a Key Vault secrets store for the scope's vault.
func keyVaultStore(_ context.Context, scope provider.Scope) (provider.Store, error) {
	if scope.VaultName == "" {
		return nil, fmt.Errorf("no Azure Key Vault specified: set --vault-name")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain Azure credentials: %w", err)
	}

	vaultURL := fmt.Sprintf("https://%s.vault.azure.net", scope.VaultName)

	client, err := azsecrets.NewClient(vaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure Key Vault client: %w", err)
	}

	return keyvault.New(keyvault.Wrap(client)), nil
}

// appConfigStore builds an App Configuration store for the scope's store.
func appConfigStore(_ context.Context, scope provider.Scope) (provider.Store, error) {
	if scope.StoreName == "" {
		return nil, fmt.Errorf("no Azure App Configuration store specified: set --store-name")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain Azure credentials: %w", err)
	}

	endpoint := fmt.Sprintf("https://%s.azconfig.io", scope.StoreName)

	client, err := azappconfig.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure App Configuration client: %w", err)
	}

	return appconfig.New(appconfig.Wrap(client)), nil
}

// Register associates the Azure Factory with provider.ProviderAzure in reg.
func Register(reg *provider.Registry) {
	reg.Register(provider.ProviderAzure, Factory{})
}

// NewRegistry returns a provider.Registry with the Azure provider registered.
func NewRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	Register(reg)

	return reg
}
