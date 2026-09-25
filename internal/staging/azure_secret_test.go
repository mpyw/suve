package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

func TestAzureSecretStrategy_BasicMethods(t *testing.T) {
	t.Parallel()

	s := staging.NewAzureSecretStrategy(nil)

	assert.Equal(t, staging.ServiceSecret, s.Service())
	assert.Equal(t, "Key Vault", s.ServiceName())
	assert.Equal(t, "secret", s.ItemName())
	// Key Vault has no force / recovery-window delete options.
	assert.False(t, s.HasDeleteOptions())
}

func TestAzureSecretStrategy_Apply(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		var created string

		store := &providermock.Store{
			CreateFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				created = name

				assert.Equal(t, "v1", value)
				assert.Equal(t, domain.ValueTypeSecret, vt)

				return domain.Version{ID: "abc"}, nil
			},
		}
		s := staging.NewAzureSecretStrategy(store)

		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationCreate, Value: new("v1")})
		require.NoError(t, err)
		assert.Equal(t, "sec", created)
	})

	t.Run("update adds a version via Put", func(t *testing.T) {
		t.Parallel()

		var putCalled bool

		store := &providermock.Store{
			PutFunc: func(_ context.Context, _, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				putCalled = true

				assert.Equal(t, "v2", value)
				assert.Equal(t, domain.ValueTypeSecret, vt)

				return domain.Version{ID: "def"}, nil
			},
		}
		s := staging.NewAzureSecretStrategy(store)

		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationUpdate, Value: new("v2")})
		require.NoError(t, err)
		assert.True(t, putCalled)
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		var deleted string

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
				deleted = name

				return nil
			},
		}
		s := staging.NewAzureSecretStrategy(store)

		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationDelete})
		require.NoError(t, err)
		assert.Equal(t, "sec", deleted)
	})

	t.Run("delete already-gone is success", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
				return secretNotFound(name)
			},
		}
		s := staging.NewAzureSecretStrategy(store)

		require.NoError(t, s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationDelete}))
	})

	t.Run("unknown operation errors", func(t *testing.T) {
		t.Parallel()

		s := staging.NewAzureSecretStrategy(&providermock.Store{})
		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.Operation("bogus")})
		require.Error(t, err)
	})
}

func TestAzureSecretStrategy_ApplyTags(t *testing.T) {
	t.Parallel()

	var added map[string]string

	var removed []string

	store := &providermock.Store{
		TagFunc: func(_ context.Context, _ string, add map[string]string) error {
			added = add

			return nil
		},
		UntagFunc: func(_ context.Context, _ string, keys []string) error {
			removed = keys

			return nil
		},
	}
	s := staging.NewAzureSecretStrategy(store)

	err := s.ApplyTags(t.Context(), "sec", staging.TagEntry{
		Add:    map[string]string{"env": "prod"},
		Remove: maputil.NewSet("old"),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod"}, added)
	assert.Equal(t, []string{"old"}, removed)
}

func TestAzureSecretStrategy_FetchLastModified(t *testing.T) {
	t.Parallel()

	mod := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)

	t.Run("returns modified time", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Modified: &mod}, nil
			},
		}
		got, err := staging.NewAzureSecretStrategy(store).FetchLastModified(t.Context(), "sec")
		require.NoError(t, err)
		assert.Equal(t, mod, got)
	})

	t.Run("not found yields ResourceNotFoundError", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound(name)
			},
		}
		_, err := staging.NewAzureSecretStrategy(store).FetchLastModified(t.Context(), "sec")
		notFoundErr := (*staging.ResourceNotFoundError)(nil)
		require.ErrorAs(t, err, &notFoundErr)
		require.ErrorIs(t, err, provider.ErrNotFound)
	})

	t.Run("nil Modified yields zero time", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "v"}, nil
			},
		}
		got, err := staging.NewAzureSecretStrategy(store).FetchLastModified(t.Context(), "sec")
		require.NoError(t, err)
		assert.True(t, got.IsZero())
	})
}

func TestAzureSecretStrategy_FetchAndTags(t *testing.T) {
	t.Parallel()

	mod := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:     name,
				Value:    "current",
				Version:  domain.Version{ID: "abc123"},
				Modified: &mod,
				Tags:     []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}
	s := staging.NewAzureSecretStrategy(store)

	fr, err := s.FetchCurrent(t.Context(), "sec")
	require.NoError(t, err)
	assert.Equal(t, "current", fr.Value)
	assert.Equal(t, "#abc123", fr.Identifier)

	tags, err := s.FetchCurrentTags(t.Context(), "sec")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod"}, tags)

	efr, err := s.FetchCurrentValue(t.Context(), "sec")
	require.NoError(t, err)
	assert.Equal(t, "current", efr.Value)
	assert.Equal(t, mod, efr.LastModified)
}

func TestAzureSecretStrategy_ParseAndResolve(t *testing.T) {
	t.Parallel()

	s := staging.NewAzureSecretStrategy(&providermock.Store{})

	t.Run("ParseName rejects version specifiers", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("sec#abc123")
		require.Error(t, err)

		_, err = s.ParseName("sec~1")
		require.Error(t, err)

		name, err := s.ParseName("sec")
		require.NoError(t, err)
		assert.Equal(t, "sec", name)
	})

	t.Run("ParseSpec detects opaque version", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("sec#abc123")
		require.NoError(t, err)
		assert.Equal(t, "sec", name)
		assert.True(t, hasVersion)

		_, hasVersion, err = s.ParseSpec("sec")
		require.NoError(t, err)
		assert.False(t, hasVersion)
	})

	t.Run("FetchVersion resolves opaque id suffix", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "#abc123", spec)

				return provider.NewVersionRef("abc123"), nil
			},
			GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
				assert.Equal(t, "abc123", ref.ID())

				return &domain.Entry{Value: "old", Version: domain.Version{ID: "abc123"}}, nil
			},
		}
		value, label, err := staging.NewAzureSecretStrategy(store).FetchVersion(t.Context(), "sec#abc123")
		require.NoError(t, err)
		assert.Equal(t, "old", value)
		assert.Equal(t, "#abc123", label)
	})

	t.Run("FetchVersion reconstructs id + shift suffix", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "#abc123~1", spec)

				return provider.NewVersionRef("older"), nil
			},
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "v", Version: domain.Version{ID: "older"}}, nil
			},
		}
		_, _, err := staging.NewAzureSecretStrategy(store).FetchVersion(t.Context(), "sec#abc123~1")
		require.NoError(t, err)
	})
}

func TestAzureSecretStrategy_ErrorPaths(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	t.Run("create error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				return domain.Version{}, boom
			},
		}
		s := staging.NewAzureSecretStrategy(store)
		err := s.Apply(t.Context(), "s", staging.Entry{Operation: staging.OperationCreate, Value: new("v")})
		require.ErrorIs(t, err, boom)
	})

	t.Run("update with nil value is a no-op", func(t *testing.T) {
		t.Parallel()

		s := staging.NewAzureSecretStrategy(&providermock.Store{})
		require.NoError(t, s.Apply(t.Context(), "s", staging.Entry{Operation: staging.OperationUpdate}))
	})

	t.Run("update error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				return domain.Version{}, boom
			},
		}
		s := staging.NewAzureSecretStrategy(store)
		err := s.Apply(t.Context(), "s", staging.Entry{Operation: staging.OperationUpdate, Value: new("v")})
		require.ErrorIs(t, err, boom)
	})

	t.Run("delete error (non-not-found) is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error { return boom },
		}
		err := staging.NewAzureSecretStrategy(store).Apply(t.Context(), "s", staging.Entry{Operation: staging.OperationDelete})
		require.ErrorIs(t, err, boom)
	})

	t.Run("ApplyTags surfaces tag and untag errors", func(t *testing.T) {
		t.Parallel()

		tagErr := &providermock.Store{TagFunc: func(_ context.Context, _ string, _ map[string]string) error { return boom }}
		err := staging.NewAzureSecretStrategy(tagErr).ApplyTags(t.Context(), "s", staging.TagEntry{Add: map[string]string{"k": "v"}})
		require.ErrorIs(t, err, boom)

		untagErr := &providermock.Store{UntagFunc: func(_ context.Context, _ string, _ []string) error { return boom }}
		err = staging.NewAzureSecretStrategy(untagErr).ApplyTags(t.Context(), "s", staging.TagEntry{Remove: maputil.NewSet("k")})
		require.ErrorIs(t, err, boom)
	})

	t.Run("FetchLastModified wraps non-not-found error", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, err := staging.NewAzureSecretStrategy(store).FetchLastModified(t.Context(), "s")
		require.ErrorIs(t, err, boom)
	})

	t.Run("FetchCurrent propagates error", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, err := staging.NewAzureSecretStrategy(store).FetchCurrent(t.Context(), "s")
		require.ErrorIs(t, err, boom)
	})

	t.Run("FetchCurrentTags: not-found and empty yield nil, error wrapped", func(t *testing.T) {
		t.Parallel()

		notFound := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound(name)
			},
		}
		tags, err := staging.NewAzureSecretStrategy(notFound).FetchCurrentTags(t.Context(), "s")
		require.NoError(t, err)
		assert.Nil(t, tags)

		empty := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{}, nil
			},
		}
		tags, err = staging.NewAzureSecretStrategy(empty).FetchCurrentTags(t.Context(), "s")
		require.NoError(t, err)
		assert.Nil(t, tags)

		errStore := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, err = staging.NewAzureSecretStrategy(errStore).FetchCurrentTags(t.Context(), "s")
		require.ErrorIs(t, err, boom)
	})

	t.Run("FetchCurrentValue: not-found maps to ResourceNotFoundError, other errors pass through", func(t *testing.T) {
		t.Parallel()

		notFound := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound(name)
			},
		}
		_, err := staging.NewAzureSecretStrategy(notFound).FetchCurrentValue(t.Context(), "s")

		var rnf *staging.ResourceNotFoundError

		require.ErrorAs(t, err, &rnf)

		errStore := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, err = staging.NewAzureSecretStrategy(errStore).FetchCurrentValue(t.Context(), "s")
		require.ErrorIs(t, err, boom)
	})

	t.Run("parse errors surface", func(t *testing.T) {
		t.Parallel()

		s := staging.NewAzureSecretStrategy(&providermock.Store{})

		_, err := s.ParseName("sec:label")
		require.Error(t, err)

		_, _, err = s.ParseSpec("sec:label")
		require.Error(t, err)

		_, _, err = s.FetchVersion(t.Context(), "sec:label")
		require.Error(t, err)
	})

	t.Run("FetchVersion propagates resolve and get errors", func(t *testing.T) {
		t.Parallel()

		resolveErr := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.VersionRef{}, boom
			},
		}
		_, _, err := staging.NewAzureSecretStrategy(resolveErr).FetchVersion(t.Context(), "sec#abc")
		require.ErrorIs(t, err, boom)

		getErr := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.NewVersionRef("abc"), nil
			},
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, _, err = staging.NewAzureSecretStrategy(getErr).FetchVersion(t.Context(), "sec#abc")
		require.ErrorIs(t, err, boom)
	})
}

func TestAzureSecretParserFactory(t *testing.T) {
	t.Parallel()

	p := staging.AzureSecretParserFactory()
	assert.Equal(t, staging.ServiceSecret, p.Service())
	assert.False(t, p.HasDeleteOptions())
}
