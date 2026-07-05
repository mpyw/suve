package keyvault

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// apiClient adapts the concrete *azsecrets.Client to the narrow Client
// interface, draining the SDK's list pagers into slices. It is the only place
// the concrete SDK client and its pagers are referenced.
type apiClient struct {
	c *azsecrets.Client
}

// Wrap adapts a concrete Key Vault secrets client to the narrow Client interface.
func Wrap(c *azsecrets.Client) Client {
	return &apiClient{c: c}
}

// Compile-time assertion that apiClient satisfies Client.
var _ Client = (*apiClient)(nil)

func (a *apiClient) GetSecret(
	ctx context.Context, name, version string,
) (azsecrets.GetSecretResponse, error) {
	return a.c.GetSecret(ctx, name, version, nil)
}

func (a *apiClient) SetSecret(
	ctx context.Context, name string, params azsecrets.SetSecretParameters,
) (azsecrets.SetSecretResponse, error) {
	return a.c.SetSecret(ctx, name, params, nil)
}

func (a *apiClient) DeleteSecret(
	ctx context.Context, name string,
) (azsecrets.DeleteSecretResponse, error) {
	return a.c.DeleteSecret(ctx, name, nil)
}

func (a *apiClient) UpdateSecretProperties(
	ctx context.Context, name, version string, params azsecrets.UpdateSecretPropertiesParameters,
) (azsecrets.UpdateSecretPropertiesResponse, error) {
	return a.c.UpdateSecretProperties(ctx, name, version, params, nil)
}

func (a *apiClient) ListSecretProperties(ctx context.Context) ([]*azsecrets.SecretProperties, error) {
	pager := a.c.NewListSecretPropertiesPager(nil)

	var out []*azsecrets.SecretProperties

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		out = append(out, page.Value...)
	}

	return out, nil
}

func (a *apiClient) ListSecretPropertiesVersions(
	ctx context.Context, name string,
) ([]*azsecrets.SecretProperties, error) {
	pager := a.c.NewListSecretPropertiesVersionsPager(name, nil)

	var out []*azsecrets.SecretProperties

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		out = append(out, page.Value...)
	}

	return out, nil
}
