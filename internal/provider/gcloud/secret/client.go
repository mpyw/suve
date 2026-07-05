package secret

import (
	"context"
	"errors"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
)

// apiClient adapts the concrete *secretmanager.Client to the narrow Client
// interface, draining the SDK's list iterators into slices. It is the only
// place the concrete SDK client and its iterators are referenced.
type apiClient struct {
	c *secretmanager.Client
}

// Wrap adapts a concrete Secret Manager client to the narrow Client interface.
func Wrap(c *secretmanager.Client) Client {
	return &apiClient{c: c}
}

// Compile-time assertion that apiClient satisfies Client.
var _ Client = (*apiClient)(nil)

func (a *apiClient) AccessSecretVersion(
	ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest,
) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	return a.c.AccessSecretVersion(ctx, req)
}

func (a *apiClient) GetSecretVersion(
	ctx context.Context, req *secretmanagerpb.GetSecretVersionRequest,
) (*secretmanagerpb.SecretVersion, error) {
	return a.c.GetSecretVersion(ctx, req)
}

func (a *apiClient) ListSecretVersions(
	ctx context.Context, req *secretmanagerpb.ListSecretVersionsRequest,
) ([]*secretmanagerpb.SecretVersion, error) {
	it := a.c.ListSecretVersions(ctx, req)

	var out []*secretmanagerpb.SecretVersion

	for {
		v, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			return nil, err
		}

		out = append(out, v)
	}

	return out, nil
}

func (a *apiClient) ListSecrets(
	ctx context.Context, req *secretmanagerpb.ListSecretsRequest,
) ([]*secretmanagerpb.Secret, error) {
	it := a.c.ListSecrets(ctx, req)

	var out []*secretmanagerpb.Secret

	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	return out, nil
}

func (a *apiClient) GetSecret(
	ctx context.Context, req *secretmanagerpb.GetSecretRequest,
) (*secretmanagerpb.Secret, error) {
	return a.c.GetSecret(ctx, req)
}

func (a *apiClient) CreateSecret(
	ctx context.Context, req *secretmanagerpb.CreateSecretRequest,
) (*secretmanagerpb.Secret, error) {
	return a.c.CreateSecret(ctx, req)
}

func (a *apiClient) AddSecretVersion(
	ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest,
) (*secretmanagerpb.SecretVersion, error) {
	return a.c.AddSecretVersion(ctx, req)
}

func (a *apiClient) DeleteSecret(ctx context.Context, req *secretmanagerpb.DeleteSecretRequest) error {
	return a.c.DeleteSecret(ctx, req)
}

func (a *apiClient) UpdateSecret(
	ctx context.Context, req *secretmanagerpb.UpdateSecretRequest,
) (*secretmanagerpb.Secret, error) {
	return a.c.UpdateSecret(ctx, req)
}
