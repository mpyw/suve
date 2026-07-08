package secret_test

import (
	"context"
	"errors"
	"testing"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// errBoom is a generic (non-gRPC) backend failure used to exercise the
// non-NotFound / non-AlreadyExists error branches.
var errBoom = errors.New("boom")

func TestResolve_Errors(t *testing.T) {
	t.Parallel()

	t.Run("invalid spec is rejected at parse time", func(t *testing.T) {
		t.Parallel()

		store := newStore(&mockClient{})
		_, err := store.Resolve(t.Context(), "my-secret", "#")
		require.Error(t, err)
	})

	t.Run("shift on secret with no versions", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
				return nil, nil
			},
		}
		store := newStore(m)

		_, err := store.Resolve(t.Context(), "my-secret", "~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no versions")
	})

	t.Run("shift from a nonexistent explicit version", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
				return []*secretmanagerpb.SecretVersion{
					{Name: versionName(1), State: secretmanagerpb.SecretVersion_ENABLED},
					{Name: versionName(2), State: secretmanagerpb.SecretVersion_ENABLED},
				}, nil
			},
		}
		store := newStore(m)

		_, err := store.Resolve(t.Context(), "my-secret", "#9~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version not found")
	})

	t.Run("shift propagates list error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
				return nil, errBoom
			},
		}
		store := newStore(m)

		_, err := store.Resolve(t.Context(), "my-secret", "~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list secret versions")
	})
}

// TestGet_BestEffortMetadata covers Get when the version-metadata and label
// lookups are unavailable: the value is still returned, Created stays nil, and
// Tags is nil (mapLabels empty branch).
func TestGet_BestEffortMetadata(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		accessFunc: func(_ context.Context, _ *secretmanagerpb.AccessSecretVersionRequest) (*secretmanagerpb.AccessSecretVersionResponse, error) {
			return &secretmanagerpb.AccessSecretVersionResponse{
				Name:    versionName(7),
				Payload: &secretmanagerpb.SecretPayload{Data: []byte("v")},
			}, nil
		},
		getVerFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
			return nil, errBoom
		},
		getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
			return &secretmanagerpb.Secret{Name: "projects/my-project/secrets/my-secret"}, nil
		},
	}
	store := newStore(m)

	entry, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.NoError(t, err)
	assert.Equal(t, "7", entry.Version.ID)
	assert.Nil(t, entry.Version.Created)
	assert.Nil(t, entry.Tags)
}

func TestHistory_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
			return nil, errBoom
		},
	}
	store := newStore(m)

	_, err := store.History(t.Context(), "my-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list secret versions")
}

// TestHistory_StateLabelsAndUnparsable covers the remaining stateLabel branches
// (disabled, unspecified) plus versionInt/lastSegment on a name without a
// numeric trailing segment.
func TestHistory_StateLabelsAndUnparsable(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVerFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretVersionsRequest) ([]*secretmanagerpb.SecretVersion, error) {
			return []*secretmanagerpb.SecretVersion{
				{Name: versionName(2), State: secretmanagerpb.SecretVersion_DISABLED},
				{Name: "weird", State: secretmanagerpb.SecretVersion_STATE_UNSPECIFIED},
				{Name: versionName(5), State: secretmanagerpb.SecretVersion_ENABLED},
			}, nil
		},
	}
	store := newStore(m)

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 3)

	// Numeric versions sort newest-first; the unparsable name (versionInt == -1)
	// sorts last.
	assert.Equal(t, "5", versions[0].ID)
	assert.Equal(t, "enabled", versions[0].Label)
	assert.Equal(t, "2", versions[1].ID)
	assert.Equal(t, "disabled", versions[1].Label)
	assert.Equal(t, "weird", versions[2].ID)
	assert.Empty(t, versions[2].Label)
}

func TestList_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context, _ *secretmanagerpb.ListSecretsRequest) ([]*secretmanagerpb.Secret, error) {
			return nil, errBoom
		},
	}
	store := newStore(m)

	_, err := store.List(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list secrets")
}

func TestCreate_Errors(t *testing.T) {
	t.Parallel()

	t.Run("create fails with a generic error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			createFunc: func(_ context.Context, _ *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				return nil, errBoom
			},
		}
		store := newStore(m)

		_, err := store.Create(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
		require.NotErrorIs(t, err, provider.ErrAlreadyExists)
		assert.Contains(t, err.Error(), "create secret")
	})

	t.Run("add version fails after create", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			createFunc: func(_ context.Context, _ *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				return &secretmanagerpb.Secret{Name: "projects/my-project/secrets/my-secret"}, nil
			},
			addFunc: func(_ context.Context, _ *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				return nil, errBoom
			},
		}
		store := newStore(m)

		_, err := store.Create(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add secret version")
	})
}

func TestPut_Errors(t *testing.T) {
	t.Parallel()

	t.Run("add fails with a non-NotFound error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			addFunc: func(_ context.Context, _ *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				return nil, status.Error(codes.PermissionDenied, "denied")
			},
		}
		store := newStore(m)

		_, err := store.Put(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add secret version")
	})

	t.Run("concurrent create is tolerated", func(t *testing.T) {
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
				return nil, status.Error(codes.AlreadyExists, "raced")
			},
		}
		store := newStore(m)

		v, err := store.Put(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
		require.NoError(t, err)
		assert.Equal(t, "1", v.ID)
		assert.Equal(t, 2, addCalls)
	})

	t.Run("create fails with a non-AlreadyExists error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			addFunc: func(_ context.Context, _ *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				return nil, status.Error(codes.NotFound, "no secret")
			},
			createFunc: func(_ context.Context, _ *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				return nil, errBoom
			},
		}
		store := newStore(m)

		_, err := store.Put(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create secret")
	})

	t.Run("add fails after create during upsert", func(t *testing.T) {
		t.Parallel()

		var addCalls int

		m := &mockClient{
			addFunc: func(_ context.Context, _ *secretmanagerpb.AddSecretVersionRequest) (*secretmanagerpb.SecretVersion, error) {
				addCalls++
				if addCalls == 1 {
					return nil, status.Error(codes.NotFound, "no secret")
				}

				return nil, errBoom
			},
			createFunc: func(_ context.Context, _ *secretmanagerpb.CreateSecretRequest) (*secretmanagerpb.Secret, error) {
				return &secretmanagerpb.Secret{Name: "projects/my-project/secrets/my-secret"}, nil
			},
		}
		store := newStore(m)

		_, err := store.Put(t.Context(), "my-secret", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "add secret version")
		assert.Equal(t, 2, addCalls)
	})
}

func TestTagUntag_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("tag with no additions is a no-op", func(t *testing.T) {
		t.Parallel()

		// No mock funcs set: if any client call happened it would panic.
		store := newStore(&mockClient{})
		require.NoError(t, store.Tag(t.Context(), "my-secret", nil))
	})

	t.Run("untag with no keys is a no-op", func(t *testing.T) {
		t.Parallel()

		store := newStore(&mockClient{})
		require.NoError(t, store.Untag(t.Context(), "my-secret", nil))
	})

	t.Run("tag propagates get-secret error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
				return nil, status.Error(codes.NotFound, "missing")
			},
		}
		store := newStore(m)

		err := store.Tag(t.Context(), "my-secret", map[string]string{"k": "v"})
		require.ErrorIs(t, err, provider.ErrNotFound)
	})

	t.Run("untag propagates get-secret error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
				return nil, errBoom
			},
		}
		store := newStore(m)

		err := store.Untag(t.Context(), "my-secret", []string{"k"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get secret")
	})

	t.Run("tag propagates update-secret error", func(t *testing.T) {
		t.Parallel()

		m := &mockClient{
			getFunc: func(_ context.Context, _ *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
				return &secretmanagerpb.Secret{Labels: map[string]string{"env": "prod"}}, nil
			},
			updateFunc: func(_ context.Context, _ *secretmanagerpb.UpdateSecretRequest) (*secretmanagerpb.Secret, error) {
				return nil, errBoom
			},
		}
		store := newStore(m)

		err := store.Tag(t.Context(), "my-secret", map[string]string{"team": "backend"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update secret labels")
	})
}
