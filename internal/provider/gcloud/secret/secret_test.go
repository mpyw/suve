package secret_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	gcloudsecret "github.com/mpyw/suve/internal/provider/gcloud/secret"
)

const testProject = "my-project"

// mockClient is a configurable in-test implementation of gcloudsecret.Client.
type mockClient struct {
	accessFunc  func(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest) (*secretmanagerpb.AccessSecretVersionResponse, error)
	getVerFunc  func(ctx context.Context, req *secretmanagerpb.GetSecretVersionRequest) (*secretmanagerpb.SecretVersion, error)
	listVerFunc func(ctx context.Context, req *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error)
	listFunc    func(ctx context.Context, req *secretmanagerpb.ListSecretsRequest) ([]*secretmanagerpb.Secret, error)
	getFunc     func(ctx context.Context, req *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error)
	createFunc  func(ctx context.Context, req *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error)
	addFunc     func(ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error)
	deleteFunc  func(ctx context.Context, req *secretmanagerpb.DeleteSecretRequest) error
	updateFunc  func(ctx context.Context, req *secretmanagerpb.UpdateSecretRequest) (*secretmanagerpb.Secret, error)
}

func (m *mockClient) AccessSecretVersion(
	ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest,
) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	return m.accessFunc(ctx, req)
}

func (m *mockClient) GetSecretVersion(
	ctx context.Context, req *secretmanagerpb.GetSecretVersionRequest,
) (*secretmanagerpb.SecretVersion, error) {
	return m.getVerFunc(ctx, req)
}

func (m *mockClient) ListSecretVersions(
	ctx context.Context, req *secretmanagerpb.ListSecretVersionsRequest,
) ([]*secretmanagerpb.SecretVersion, error) {
	return m.listVerFunc(ctx, req)
}

func (m *mockClient) ListSecrets(
	ctx context.Context, req *secretmanagerpb.ListSecretsRequest,
) ([]*secretmanagerpb.Secret, error) {
	return m.listFunc(ctx, req)
}

func (m *mockClient) GetSecret(
	ctx context.Context, req *secretmanagerpb.GetSecretRequest,
) (*secretmanagerpb.Secret, error) {
	return m.getFunc(ctx, req)
}

func (m *mockClient) CreateSecret(
	ctx context.Context, req *secretmanagerpb.CreateSecretRequest,
) (*secretmanagerpb.Secret, error) {
	return m.createFunc(ctx, req)
}

func (m *mockClient) AddSecretVersion(
	ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest,
) (*secretmanagerpb.SecretVersion, error) {
	return m.addFunc(ctx, req)
}

func (m *mockClient) DeleteSecret(ctx context.Context, req *secretmanagerpb.DeleteSecretRequest) error {
	return m.deleteFunc(ctx, req)
}

func (m *mockClient) UpdateSecret(
	ctx context.Context, req *secretmanagerpb.UpdateSecretRequest,
) (*secretmanagerpb.Secret, error) {
	return m.updateFunc(ctx, req)
}

func versionName(n int) string {
	return "projects/" + testProject + "/secrets/my-secret/versions/" + strconv.Itoa(n)
}

func newStore(m *mockClient) *gcloudsecret.Store {
	return gcloudsecret.New(m, testProject)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	t.Run("latest when empty spec", func(t *testing.T) {
		t.Parallel()

		store := newStore(&mockClient{})
		ref, err := store.Resolve(t.Context(), "my-secret", "")
		require.NoError(t, err)
		assert.True(t, ref.IsLatest())
	})

	t.Run("explicit version needs no listing", func(t *testing.T) {
		t.Parallel()

		store := newStore(&mockClient{})
		ref, err := store.Resolve(t.Context(), "my-secret", "#3")
		require.NoError(t, err)
		assert.Equal(t, "3", ref.ID())
	})

	t.Run("shift walks enabled versions newest first", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
				return []*secretmanagerpb.SecretVersion{
					{Name: versionName(1), State: secretmanagerpb.SecretVersion_ENABLED},
					{Name: versionName(3), State: secretmanagerpb.SecretVersion_ENABLED},
					{Name: versionName(2), State: secretmanagerpb.SecretVersion_DISABLED},
				}, nil
			},
		}
		store := newStore(m)

		// ~1 from latest ENABLED (3) -> next enabled is 1 (2 is disabled, skipped).
		ref, err := store.Resolve(t.Context(), "my-secret", "~1")
		require.NoError(t, err)
		assert.Equal(t, "1", ref.ID())
	})

	t.Run("shift from explicit version", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
				return []*secretmanagerpb.SecretVersion{
					{Name: versionName(1), State: secretmanagerpb.SecretVersion_ENABLED},
					{Name: versionName(2), State: secretmanagerpb.SecretVersion_ENABLED},
					{Name: versionName(3), State: secretmanagerpb.SecretVersion_ENABLED},
				}, nil
			},
		}
		store := newStore(m)

		ref, err := store.Resolve(t.Context(), "my-secret", "#3~2")
		require.NoError(t, err)
		assert.Equal(t, "1", ref.ID())
	})

	t.Run("shift out of range", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
				return []*secretmanagerpb.SecretVersion{
					{Name: versionName(1), State: secretmanagerpb.SecretVersion_ENABLED},
				}, nil
			},
		}
		store := newStore(m)

		_, err := store.Resolve(t.Context(), "my-secret", "~5")
		require.Error(t, err)
	})

	t.Run("label spec rejected cleanly", func(t *testing.T) {
		t.Parallel()

		store := newStore(&mockClient{})
		_, err := store.Resolve(t.Context(), "my-secret", ":latest")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "staging labels are not supported")
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	m := &mockClient{
		accessFunc: func(_ context.Context, req *secretmanagerpb.AccessSecretVersionRequest) (*secretmanagerpb.AccessSecretVersionResponse, error) {
			assert.Equal(t, "projects/my-project/secrets/my-secret/versions/latest", req.GetName())

			return &secretmanagerpb.AccessSecretVersionResponse{
				Name:    versionName(5),
				Payload: &secretmanagerpb.SecretPayload{Data: []byte("s3cr3t")},
			}, nil
		},
		getVerFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
			return &secretmanagerpb.SecretVersion{
				Name:       versionName(5),
				State:      secretmanagerpb.SecretVersion_ENABLED,
				CreateTime: timestamppb.New(created),
			}, nil
		},
		getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
			return &secretmanagerpb.Secret{
				Name:   "projects/my-project/secrets/my-secret",
				Labels: map[string]string{"env": "prod", "team": "backend"},
			}, nil
		},
	}
	store := newStore(m)

	entry, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", entry.Name)
	assert.Equal(t, "s3cr3t", entry.Value)
	assert.Equal(t, domain.ValueTypeSecret, entry.Type)
	assert.Equal(t, "5", entry.Version.ID)
	require.NotNil(t, entry.Version.Created)
	assert.Equal(t, created, *entry.Version.Created)
	assert.Nil(t, entry.Extra)
	assert.Equal(t, []domain.Tag{{Key: "env", Value: "prod"}, {Key: "team", Value: "backend"}}, entry.Tags)
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		accessFunc: func(_ context.Context, _ *secretmanagerpb.AccessSecretVersionRequest) (*secretmanagerpb.AccessSecretVersionResponse, error) {
			return nil, status.Error(codes.NotFound, "not found")
		},
	}
	store := newStore(m)

	_, err := store.Get(t.Context(), "missing", provider.VersionRef{})
	require.ErrorIs(t, err, provider.ErrNotFound)
}

func TestHistory(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
			return []*secretmanagerpb.SecretVersion{
				{Name: versionName(1), State: secretmanagerpb.SecretVersion_DESTROYED},
				{Name: versionName(2), State: secretmanagerpb.SecretVersion_ENABLED},
			}, nil
		},
	}
	store := newStore(m)

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	// Newest first: version 2 (enabled) then 1 (destroyed).
	assert.Equal(t, "2", versions[0].ID)
	assert.Equal(t, "enabled", versions[0].Label)
	assert.Equal(t, "1", versions[1].ID)
	assert.Equal(t, "destroyed", versions[1].Label)
}

func TestList(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context, req *secretmanagerpb.ListSecretsRequest) ([]*secretmanagerpb.Secret, error) {
			assert.Equal(t, "projects/my-project", req.GetParent())

			return []*secretmanagerpb.Secret{
				{Name: "projects/my-project/secrets/alpha"},
				{Name: "projects/my-project/secrets/beta"},
			}, nil
		},
	}
	store := newStore(m)

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestCreate(t *testing.T) {
	t.Parallel()

	t.Run("creates secret and adds first version", func(t *testing.T) {
		t.Parallel()

		var createCalled, addCalled bool

		m := &mockClient{
			createFunc: func(_ context.Context, req *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				createCalled = true

				assert.Equal(t, "my-secret", req.GetSecretId())
				assert.NotNil(t, req.GetSecret().GetReplication().GetAutomatic())

				return &secretmanagerpb.Secret{Name: "projects/my-project/secrets/my-secret"}, nil
			},
			addFunc: func(_ context.Context, req *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				addCalled = true

				assert.Equal(t, []byte("value"), req.GetPayload().GetData())

				return &secretmanagerpb.SecretVersion{Name: versionName(1)}, nil
			},
		}
		store := newStore(m)

		v, err := store.Create(t.Context(), "my-secret", "value", domain.ValueTypeSecret, "")
		require.NoError(t, err)
		assert.Equal(t, "1", v.ID)
		assert.True(t, createCalled)
		assert.True(t, addCalled)
	})

	t.Run("already exists", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			createFunc: func(_ context.Context, _ *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				return nil, status.Error(codes.AlreadyExists, "exists")
			},
		}
		store := newStore(m)

		_, err := store.Create(t.Context(), "my-secret", "value", domain.ValueTypeSecret, "")
		require.ErrorIs(t, err, provider.ErrAlreadyExists)
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	t.Run("adds version to existing secret", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			addFunc: func(_ context.Context, _ *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				return &secretmanagerpb.SecretVersion{Name: versionName(4)}, nil
			},
		}
		store := newStore(m)

		v, err := store.Put(t.Context(), "my-secret", "value", domain.ValueTypeSecret, "")
		require.NoError(t, err)
		assert.Equal(t, "4", v.ID)
	})

	t.Run("creates secret when missing then adds", func(t *testing.T) {
		t.Parallel()

		var addCalls int

		m := &mockClient{
			addFunc: func(_ context.Context, _ *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				addCalls++
				if addCalls == 1 {
					return nil, status.Error(codes.NotFound, "no secret")
				}

				return &secretmanagerpb.SecretVersion{Name: versionName(1)}, nil
			},
			createFunc: func(_ context.Context, _ *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				return &secretmanagerpb.Secret{Name: "projects/my-project/secrets/my-secret"}, nil
			},
		}
		store := newStore(m)

		v, err := store.Put(t.Context(), "my-secret", "value", domain.ValueTypeSecret, "")
		require.NoError(t, err)
		assert.Equal(t, "1", v.ID)
		assert.Equal(t, 2, addCalls)
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("permanent delete", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			deleteFunc: func(_ context.Context, req *secretmanagerpb.DeleteSecretRequest) error {
				assert.Equal(t, "projects/my-project/secrets/my-secret", req.GetName())

				return nil
			},
		}
		store := newStore(m)

		require.NoError(t, store.Delete(t.Context(), "my-secret"))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			deleteFunc: func(_ context.Context, _ *secretmanagerpb.DeleteSecretRequest) error {
				return status.Error(codes.NotFound, "no secret")
			},
		}
		store := newStore(m)

		require.ErrorIs(t, store.Delete(t.Context(), "my-secret"), provider.ErrNotFound)
	})
}

func TestTagUntag(t *testing.T) {
	t.Parallel()

	t.Run("tag merges labels", func(t *testing.T) {
		t.Parallel()

		var written map[string]string

		m := &mockClient{
			getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
				return &secretmanagerpb.Secret{Labels: map[string]string{"env": "prod"}}, nil
			},
			updateFunc: func(_ context.Context, req *secretmanagerpb.UpdateSecretRequest) (*secretmanagerpb.Secret, error) {
				written = req.GetSecret().GetLabels()
				assert.Equal(t, []string{"labels"}, req.GetUpdateMask().GetPaths())

				return req.GetSecret(), nil
			},
		}
		store := newStore(m)

		require.NoError(t, store.Tag(t.Context(), "my-secret", map[string]string{"team": "backend"}))
		assert.Equal(t, map[string]string{"env": "prod", "team": "backend"}, written)
	})

	t.Run("untag removes labels", func(t *testing.T) {
		t.Parallel()

		var written map[string]string

		m := &mockClient{
			getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
				return &secretmanagerpb.Secret{Labels: map[string]string{"env": "prod", "team": "backend"}}, nil
			},
			updateFunc: func(_ context.Context, req *secretmanagerpb.UpdateSecretRequest) (*secretmanagerpb.Secret, error) {
				written = req.GetSecret().GetLabels()

				return req.GetSecret(), nil
			},
		}
		store := newStore(m)

		require.NoError(t, store.Untag(t.Context(), "my-secret", []string{"team"}))
		assert.Equal(t, map[string]string{"env": "prod"}, written)
	})
}
