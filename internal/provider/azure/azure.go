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
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/azure/keyvault"
)

// AppConfigConnStringEnvVar is the environment variable that, when set, points
// the App Configuration adapter at an emulator via a connection string
// (Endpoint=...;Id=...;Secret=...) instead of the real https://<store>.azconfig.io
// endpoint with DefaultAzureCredential. It is an emulator-only seam for offline
// e2e tests (see the azure-appconfig service in compose.yaml and the `e2e-azure`
// make target); it permits HMAC credentials over plain HTTP, so it must never be
// set against a production store.
const AppConfigConnStringEnvVar = "SUVE_AZURE_APPCONFIG_CONNECTION_STRING"

// KeyVaultEndpointEnvVar is the environment variable that, when set, points the
// Key Vault adapter at an emulator (e.g. https://localhost:8443) instead of the
// real https://<vault>.vault.azure.net endpoint with DefaultAzureCredential. It
// is an emulator-only seam for offline e2e tests (see the azure-keyvault service
// in compose.yaml and the `e2e-azure-keyvault` make target); it disables TLS
// verification and token challenge checks and sends a dummy token, so it must
// never be set against a production vault.
const KeyVaultEndpointEnvVar = "SUVE_AZURE_KEYVAULT_ENDPOINT"

// emulatorCredential is a no-op azcore.TokenCredential for the Key Vault
// emulator seam. Emulators (e.g. lowkey-vault) check for the presence of a
// bearer token but ignore its value; this returns a static one. Never used
// against real Azure.
type emulatorCredential struct{}

func (emulatorCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	//nolint:gosec // "suve-emulator" is a dummy token for the emulator, not a real credential
	return azcore.AccessToken{Token: "suve-emulator", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

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

	// Emulator seam: an endpoint override (offline e2e) targets a local Key
	// Vault emulator with a self-signed cert and no real auth, so it disables
	// TLS verification and token challenge checks and sends a dummy token.
	if endpoint := os.Getenv(KeyVaultEndpointEnvVar); endpoint != "" {
		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // emulator-only seam
			},
		}
		opts := &azsecrets.ClientOptions{
			ClientOptions:                        azcore.ClientOptions{Transport: httpClient},
			DisableChallengeResourceVerification: true,
		}

		client, err := azsecrets.NewClient(endpoint, emulatorCredential{}, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure Key Vault client for emulator: %w", err)
		}

		return keyvault.New(keyvault.Wrap(client)), nil
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

	// Emulator seam: a connection string (offline e2e) bypasses the real
	// endpoint + DefaultAzureCredential and allows HMAC auth over plain HTTP.
	if connStr := os.Getenv(AppConfigConnStringEnvVar); connStr != "" {
		opts := &azappconfig.ClientOptions{
			ClientOptions: azcore.ClientOptions{InsecureAllowCredentialWithHTTP: true},
		}

		client, err := azappconfig.NewClientFromConnectionString(connStr, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure App Configuration client from connection string: %w", err)
		}

		return appconfig.New(appconfig.Wrap(client)), nil
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
