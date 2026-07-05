package staging_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

func TestGoogleCloudSecretStrategy_BasicMethods(t *testing.T) {
	t.Parallel()

	s := staging.NewGoogleCloudSecretStrategy(nil)

	assert.Equal(t, staging.ServiceSecret, s.Service())
	assert.Equal(t, "Secret Manager", s.ServiceName())
	assert.Equal(t, "secret", s.ItemName())
	// Google Cloud has no force / recovery-window delete options.
	assert.False(t, s.HasDeleteOptions())
}

func TestGoogleCloudSecretStrategy_Apply(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		var created string

		store := &providermock.Store{
			CreateFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				created = name

				assert.Equal(t, "v1", value)
				assert.Equal(t, domain.ValueTypeSecret, vt)

				return domain.Version{ID: "1"}, nil
			},
		}
		s := staging.NewGoogleCloudSecretStrategy(store)

		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationCreate, Value: lo.ToPtr("v1")})
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

				return domain.Version{ID: "2"}, nil
			},
		}
		s := staging.NewGoogleCloudSecretStrategy(store)

		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v2")})
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
		s := staging.NewGoogleCloudSecretStrategy(store)

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
		s := staging.NewGoogleCloudSecretStrategy(store)

		require.NoError(t, s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.OperationDelete}))
	})

	t.Run("unknown operation errors", func(t *testing.T) {
		t.Parallel()

		s := staging.NewGoogleCloudSecretStrategy(&providermock.Store{})
		err := s.Apply(t.Context(), "sec", staging.Entry{Operation: staging.Operation("bogus")})
		require.Error(t, err)
	})
}

func TestGoogleCloudSecretStrategy_FetchLastModified(t *testing.T) {
	t.Parallel()

	mod := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)

	t.Run("returns modified time", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Modified: &mod}, nil
			},
		}
		got, err := staging.NewGoogleCloudSecretStrategy(store).FetchLastModified(t.Context(), "sec")
		require.NoError(t, err)
		assert.Equal(t, mod, got)
	})

	t.Run("not found yields zero time, no error", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound(name)
			},
		}
		got, err := staging.NewGoogleCloudSecretStrategy(store).FetchLastModified(t.Context(), "sec")
		require.NoError(t, err)
		assert.True(t, got.IsZero())
	})
}

func TestGoogleCloudSecretStrategy_FetchAndTags(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "current",
				Version: domain.Version{ID: "3"},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}
	s := staging.NewGoogleCloudSecretStrategy(store)

	fr, err := s.FetchCurrent(t.Context(), "sec")
	require.NoError(t, err)
	assert.Equal(t, "current", fr.Value)
	assert.Equal(t, "#3", fr.Identifier)

	tags, err := s.FetchCurrentTags(t.Context(), "sec")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod"}, tags)

	efr, err := s.FetchCurrentValue(t.Context(), "sec")
	require.NoError(t, err)
	assert.Equal(t, "current", efr.Value)
}

func TestGoogleCloudSecretStrategy_ApplyTags(t *testing.T) {
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
	s := staging.NewGoogleCloudSecretStrategy(store)

	err := s.ApplyTags(t.Context(), "sec", staging.TagEntry{
		Add:    map[string]string{"env": "prod"},
		Remove: maputil.NewSet("old"),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod"}, added)
	assert.Equal(t, []string{"old"}, removed)
}

func TestGoogleCloudSecretStrategy_ParseAndResolve(t *testing.T) {
	t.Parallel()

	s := staging.NewGoogleCloudSecretStrategy(&providermock.Store{})

	t.Run("ParseName rejects version specifier", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("sec#3")
		require.Error(t, err)

		name, err := s.ParseName("sec")
		require.NoError(t, err)
		assert.Equal(t, "sec", name)
	})

	t.Run("ParseSpec detects version", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("sec#2")
		require.NoError(t, err)
		assert.Equal(t, "sec", name)
		assert.True(t, hasVersion)

		_, hasVersion, err = s.ParseSpec("sec")
		require.NoError(t, err)
		assert.False(t, hasVersion)
	})

	t.Run("FetchVersion resolves and gets", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "#1", spec)

				return provider.NewVersionRef("1"), nil
			},
			GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
				assert.Equal(t, "1", ref.ID())

				return &domain.Entry{Value: "old", Version: domain.Version{ID: "1"}}, nil
			},
		}
		value, label, err := staging.NewGoogleCloudSecretStrategy(store).FetchVersion(t.Context(), "sec#1")
		require.NoError(t, err)
		assert.Equal(t, "old", value)
		assert.Equal(t, "#1", label)
	})
}

// GoogleCloudSecretParserFactory yields a parser-only strategy.
func TestGoogleCloudSecretParserFactory(t *testing.T) {
	t.Parallel()

	p := staging.GoogleCloudSecretParserFactory()
	assert.Equal(t, staging.ServiceSecret, p.Service())
	assert.False(t, p.HasDeleteOptions())
}
