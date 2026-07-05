package appconfig_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
)

func notFound() error {
	return &azcore.ResponseError{StatusCode: http.StatusNotFound}
}

func preconditionFailed() error {
	return &azcore.ResponseError{StatusCode: http.StatusPreconditionFailed}
}

// mockClient is a configurable in-test implementation of appconfig.Client.
type mockClient struct {
	getFunc    func(ctx context.Context, key string) (azappconfig.GetSettingResponse, error)
	setFunc    func(ctx context.Context, key, value string) (azappconfig.SetSettingResponse, error)
	addFunc    func(ctx context.Context, key, value string) (azappconfig.AddSettingResponse, error)
	deleteFunc func(ctx context.Context, key string) (azappconfig.DeleteSettingResponse, error)
	listFunc   func(ctx context.Context) ([]azappconfig.Setting, error)
}

func (m *mockClient) GetSetting(ctx context.Context, key string) (azappconfig.GetSettingResponse, error) {
	return m.getFunc(ctx, key)
}

func (m *mockClient) SetSetting(ctx context.Context, key, value string) (azappconfig.SetSettingResponse, error) {
	return m.setFunc(ctx, key, value)
}

func (m *mockClient) AddSetting(ctx context.Context, key, value string) (azappconfig.AddSettingResponse, error) {
	return m.addFunc(ctx, key, value)
}

func (m *mockClient) DeleteSetting(ctx context.Context, key string) (azappconfig.DeleteSettingResponse, error) {
	return m.deleteFunc(ctx, key)
}

func (m *mockClient) ListSettings(ctx context.Context) ([]azappconfig.Setting, error) {
	return m.listFunc(ctx)
}

func TestResolve_BareNameLatest(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{})
	ref, err := store.Resolve(t.Context(), "my-key", "")
	require.NoError(t, err)
	assert.True(t, ref.IsLatest())
}

func TestResolve_RejectsVersionSpecs(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{})

	for _, spec := range []string{"#1", "#abc", "~1", ":prod"} {
		_, err := store.Resolve(t.Context(), "my-key", spec)
		require.Error(t, err, "spec=%q", spec)
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, key string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("30"),
				Tags:  map[string]string{"env": "prod"},
			}}, nil
		},
	}
	store := appconfig.New(m)

	entry, err := store.Get(t.Context(), "app/timeout", provider.VersionRef{})
	require.NoError(t, err)
	assert.Equal(t, "app/timeout", entry.Name)
	assert.Equal(t, "30", entry.Value)
	assert.Equal(t, domain.ValueTypePlaintext, entry.Type)
	assert.Empty(t, entry.Version.ID) // unversioned
	assert.Equal(t, []domain.Tag{{Key: "env", Value: "prod"}}, entry.Tags)
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{}, notFound()
		},
	}
	store := appconfig.New(m)

	_, err := store.Get(t.Context(), "missing", provider.VersionRef{})
	require.ErrorIs(t, err, provider.ErrNotFound)
}

func TestHistory_Unsupported(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{})

	versions, err := store.History(t.Context(), "my-key")
	require.Error(t, err)
	assert.Nil(t, versions)
	assert.Contains(t, err.Error(), "does not support versions")
}

func TestList(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context) ([]azappconfig.Setting, error) {
			return []azappconfig.Setting{
				{Key: lo.ToPtr("beta")},
				{Key: lo.ToPtr("alpha")},
				{Key: lo.ToPtr("beta")}, // duplicate across labels
			}, nil
		},
	}
	store := appconfig.New(m)

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestCreate_AlreadyExists(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		addFunc: func(_ context.Context, _, _ string) (azappconfig.AddSettingResponse, error) {
			return azappconfig.AddSettingResponse{}, preconditionFailed()
		},
	}
	store := appconfig.New(m)

	_, err := store.Create(t.Context(), "existing", "v", domain.ValueTypePlaintext, "")
	require.ErrorIs(t, err, provider.ErrAlreadyExists)
}

func TestCreate_New(t *testing.T) {
	t.Parallel()

	var added string

	m := &mockClient{
		addFunc: func(_ context.Context, key, value string) (azappconfig.AddSettingResponse, error) {
			added = key

			assert.Equal(t, "v", value)

			return azappconfig.AddSettingResponse{}, nil
		},
	}
	store := appconfig.New(m)

	version, err := store.Create(t.Context(), "new-key", "v", domain.ValueTypePlaintext, "")
	require.NoError(t, err)
	assert.Equal(t, "new-key", added)
	assert.Empty(t, version.ID) // unversioned
}

func TestPut(t *testing.T) {
	t.Parallel()

	var setKey, setVal string

	m := &mockClient{
		setFunc: func(_ context.Context, key, value string) (azappconfig.SetSettingResponse, error) {
			setKey, setVal = key, value

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m)

	version, err := store.Put(t.Context(), "app/timeout", "60", domain.ValueTypePlaintext, "")
	require.NoError(t, err)
	assert.Equal(t, "app/timeout", setKey)
	assert.Equal(t, "60", setVal)
	assert.Empty(t, version.ID)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	var deleted string

	m := &mockClient{
		deleteFunc: func(_ context.Context, key string) (azappconfig.DeleteSettingResponse, error) {
			deleted = key

			return azappconfig.DeleteSettingResponse{}, nil
		},
	}
	store := appconfig.New(m)

	require.NoError(t, store.Delete(t.Context(), "app/timeout"))
	assert.Equal(t, "app/timeout", deleted)
}

func TestTagUntag_Unsupported(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{})

	// Non-empty mutation is declined with a clear error.
	require.ErrorIs(t, store.Tag(t.Context(), "k", map[string]string{"env": "prod"}), appconfig.ErrTagsUnsupported)
	require.ErrorIs(t, store.Untag(t.Context(), "k", []string{"env"}), appconfig.ErrTagsUnsupported)

	// Empty mutations are no-ops.
	require.NoError(t, store.Tag(t.Context(), "k", nil))
	require.NoError(t, store.Untag(t.Context(), "k", nil))
}

// Compile-time assertion that the mock satisfies the adapter's Client interface.
var _ appconfig.Client = (*mockClient)(nil)
