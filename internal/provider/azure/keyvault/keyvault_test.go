package keyvault_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/keyvault"
)

const vaultBase = "https://myvault.vault.azure.net/secrets/"

// secretID builds a Key Vault secret ID URL for the given name and version.
func secretID(name, version string) *azsecrets.ID {
	id := azsecrets.ID(vaultBase + name + "/" + version)

	return &id
}

func notFound() error {
	return &azcore.ResponseError{StatusCode: http.StatusNotFound}
}

// Test-local aliases keep the mock signatures within the line-length limit.
type (
	updParams = azsecrets.UpdateSecretPropertiesParameters
	updResp   = azsecrets.UpdateSecretPropertiesResponse
)

// mockClient is a configurable in-test implementation of keyvault.Client.
type mockClient struct {
	getFunc      func(ctx context.Context, name, version string) (azsecrets.GetSecretResponse, error)
	setFunc      func(ctx context.Context, name string, params azsecrets.SetSecretParameters) (azsecrets.SetSecretResponse, error)
	deleteFunc   func(ctx context.Context, name string) (azsecrets.DeleteSecretResponse, error)
	updateFunc   func(ctx context.Context, name, version string, params updParams) (updResp, error)
	listFunc     func(ctx context.Context) ([]*azsecrets.SecretProperties, error)
	listVersFunc func(ctx context.Context, name string) ([]*azsecrets.SecretProperties, error)
}

func (m *mockClient) GetSecret(ctx context.Context, name, version string) (azsecrets.GetSecretResponse, error) {
	return m.getFunc(ctx, name, version)
}

func (m *mockClient) SetSecret(
	ctx context.Context, name string, params azsecrets.SetSecretParameters,
) (azsecrets.SetSecretResponse, error) {
	return m.setFunc(ctx, name, params)
}

func (m *mockClient) DeleteSecret(ctx context.Context, name string) (azsecrets.DeleteSecretResponse, error) {
	return m.deleteFunc(ctx, name)
}

func (m *mockClient) UpdateSecretProperties(
	ctx context.Context, name, version string, params updParams,
) (updResp, error) {
	return m.updateFunc(ctx, name, version, params)
}

func (m *mockClient) ListSecretProperties(ctx context.Context) ([]*azsecrets.SecretProperties, error) {
	return m.listFunc(ctx)
}

func (m *mockClient) ListSecretPropertiesVersions(
	ctx context.Context, name string,
) ([]*azsecrets.SecretProperties, error) {
	return m.listVersFunc(ctx, name)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	t.Run("current when empty spec", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{})
		ref, err := store.Resolve(t.Context(), "my-secret", "")
		require.NoError(t, err)
		assert.True(t, ref.IsLatest())
	})

	t.Run("explicit version id needs no listing", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{})
		ref, err := store.Resolve(t.Context(), "my-secret", "#abc123")
		require.NoError(t, err)
		assert.Equal(t, "abc123", ref.ID())
	})

	t.Run("shift walks versions newest first", func(t *testing.T) {
		t.Parallel()

		older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		newer := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		m := &mockClient{
			listVersFunc: func(_ context.Context, _ string) ([]*azsecrets.SecretProperties, error) {
				return []*azsecrets.SecretProperties{
					{ID: secretID("my-secret", "old"), Attributes: &azsecrets.SecretAttributes{Created: &older}},
					{ID: secretID("my-secret", "new"), Attributes: &azsecrets.SecretAttributes{Created: &newer}},
				}, nil
			},
		}
		store := keyvault.New(m)

		// ~1 from current => the second-newest version ("old").
		ref, err := store.Resolve(t.Context(), "my-secret", "~1")
		require.NoError(t, err)
		assert.Equal(t, "old", ref.ID())
	})

	t.Run("label spec rejected", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{})
		_, err := store.Resolve(t.Context(), "my-secret", ":latest")
		require.Error(t, err)
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	m := &mockClient{
		getFunc: func(_ context.Context, name, version string) (azsecrets.GetSecretResponse, error) {
			assert.Empty(t, version)

			return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{
				ID:         secretID(name, "v1"),
				Value:      lo.ToPtr("s3cr3t"),
				Attributes: &azsecrets.SecretAttributes{Created: &created, Enabled: lo.ToPtr(true)},
				Tags:       map[string]*string{"env": lo.ToPtr("prod")},
			}}, nil
		},
	}
	store := keyvault.New(m)

	entry, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", entry.Name)
	assert.Equal(t, "s3cr3t", entry.Value)
	assert.Equal(t, domain.ValueTypeSecret, entry.Type)
	assert.Equal(t, "v1", entry.Version.ID)
	// State carries the per-version enable/disable; StagingLabels is not a Key Vault concept.
	assert.Equal(t, "enabled", entry.Version.State)
	assert.Empty(t, entry.Version.StagingLabels)
	assert.Equal(t, []domain.Tag{{Key: "env", Value: "prod"}}, entry.Tags)
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azsecrets.GetSecretResponse, error) {
			return azsecrets.GetSecretResponse{}, notFound()
		},
	}
	store := keyvault.New(m)

	_, err := store.Get(t.Context(), "missing", provider.VersionRef{})
	require.ErrorIs(t, err, provider.ErrNotFound)
}

func TestHistory(t *testing.T) {
	t.Parallel()

	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	m := &mockClient{
		listVersFunc: func(_ context.Context, _ string) ([]*azsecrets.SecretProperties, error) {
			return []*azsecrets.SecretProperties{
				{ID: secretID("my-secret", "old"), Attributes: &azsecrets.SecretAttributes{Created: &older, Enabled: lo.ToPtr(false)}},
				{ID: secretID("my-secret", "new"), Attributes: &azsecrets.SecretAttributes{Created: &newer, Enabled: lo.ToPtr(true)}},
			}, nil
		},
	}
	store := keyvault.New(m)

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	// Newest first.
	assert.Equal(t, "new", versions[0].ID)
	assert.Equal(t, "enabled", versions[0].State)
	assert.Empty(t, versions[0].StagingLabels)
	assert.Equal(t, "old", versions[1].ID)
	assert.Equal(t, "disabled", versions[1].State)
	assert.Empty(t, versions[1].StagingLabels)
}

func TestList(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context) ([]*azsecrets.SecretProperties, error) {
			return []*azsecrets.SecretProperties{
				{ID: secretID("alpha", "v1")},
				{ID: secretID("beta", "v1")},
			}, nil
		},
	}
	store := keyvault.New(m)

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestCreate_AlreadyExists(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, name, _ string) (azsecrets.GetSecretResponse, error) {
			// Secret exists.
			return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{ID: secretID(name, "v1")}}, nil
		},
	}
	store := keyvault.New(m)

	_, err := store.Create(t.Context(), "existing", "value", domain.ValueTypeSecret, "")
	require.ErrorIs(t, err, provider.ErrAlreadyExists)
}

func TestCreate_NewSecret(t *testing.T) {
	t.Parallel()

	var setCalled bool

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azsecrets.GetSecretResponse, error) {
			return azsecrets.GetSecretResponse{}, notFound()
		},
		setFunc: func(_ context.Context, name string, params azsecrets.SetSecretParameters) (azsecrets.SetSecretResponse, error) {
			setCalled = true

			assert.Equal(t, "new-value", lo.FromPtr(params.Value))

			return azsecrets.SetSecretResponse{Secret: azsecrets.Secret{ID: secretID(name, "v1")}}, nil
		},
	}
	store := keyvault.New(m)

	version, err := store.Create(t.Context(), "new-secret", "new-value", domain.ValueTypeSecret, "")
	require.NoError(t, err)
	assert.True(t, setCalled)
	assert.Equal(t, "v1", version.ID)
}

func TestPut(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		setFunc: func(_ context.Context, name string, params azsecrets.SetSecretParameters) (azsecrets.SetSecretResponse, error) {
			assert.Equal(t, "v", lo.FromPtr(params.Value))

			return azsecrets.SetSecretResponse{Secret: azsecrets.Secret{ID: secretID(name, "v2")}}, nil
		},
	}
	store := keyvault.New(m)

	version, err := store.Put(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
	require.NoError(t, err)
	assert.Equal(t, "v2", version.ID)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	var deleted string

	m := &mockClient{
		deleteFunc: func(_ context.Context, name string) (azsecrets.DeleteSecretResponse, error) {
			deleted = name

			return azsecrets.DeleteSecretResponse{}, nil
		},
	}
	store := keyvault.New(m)

	require.NoError(t, store.Delete(t.Context(), "my-secret"))
	assert.Equal(t, "my-secret", deleted)
}

func TestTag(t *testing.T) {
	t.Parallel()

	var written map[string]*string

	m := &mockClient{
		getFunc: func(_ context.Context, name, _ string) (azsecrets.GetSecretResponse, error) {
			return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{
				ID:   secretID(name, "v1"),
				Tags: map[string]*string{"existing": lo.ToPtr("keep")},
			}}, nil
		},
		updateFunc: func(_ context.Context, _, version string, params updParams) (updResp, error) {
			assert.Empty(t, version) // current version

			written = params.Tags

			return updResp{}, nil
		},
	}
	store := keyvault.New(m)

	require.NoError(t, store.Tag(t.Context(), "my-secret", map[string]string{"env": "prod"}))
	assert.Equal(t, "keep", lo.FromPtr(written["existing"]))
	assert.Equal(t, "prod", lo.FromPtr(written["env"]))
}

func TestUntag(t *testing.T) {
	t.Parallel()

	var written map[string]*string

	m := &mockClient{
		getFunc: func(_ context.Context, name, _ string) (azsecrets.GetSecretResponse, error) {
			return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{
				ID:   secretID(name, "v1"),
				Tags: map[string]*string{"env": lo.ToPtr("prod"), "team": lo.ToPtr("backend")},
			}}, nil
		},
		updateFunc: func(_ context.Context, _, _ string, params updParams) (updResp, error) {
			written = params.Tags

			return updResp{}, nil
		},
	}
	store := keyvault.New(m)

	require.NoError(t, store.Untag(t.Context(), "my-secret", []string{"team"}))

	_, hasTeam := written["team"]
	assert.False(t, hasTeam)
	assert.Equal(t, "prod", lo.FromPtr(written["env"]))
}

// Compile-time assertion that the mock satisfies the adapter's Client interface.
var _ keyvault.Client = (*mockClient)(nil)
