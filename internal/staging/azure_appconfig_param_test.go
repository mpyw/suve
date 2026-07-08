package staging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

func TestAzureAppConfigParamStrategy_BasicMethods(t *testing.T) {
	t.Parallel()

	s := staging.NewAzureAppConfigParamStrategy(nil)

	assert.Equal(t, staging.ServiceParam, s.Service())
	assert.Equal(t, "App Configuration", s.ServiceName())
	assert.Equal(t, "setting", s.ItemName())
	// App Configuration has no delete options.
	assert.False(t, s.HasDeleteOptions())
}

func TestAzureAppConfigParamStrategy_Apply(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		var created string

		store := &providermock.Store{
			CreateFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				created = name

				assert.Equal(t, "v1", value)
				assert.Equal(t, domain.ValueTypePlaintext, vt)

				return domain.Version{}, nil
			},
		}
		s := staging.NewAzureAppConfigParamStrategy(store)

		err := s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationCreate, Value: lo.ToPtr("v1")})
		require.NoError(t, err)
		assert.Equal(t, "cfg", created)
	})

	t.Run("update overwrites via Put (last-write-wins)", func(t *testing.T) {
		t.Parallel()

		var putCalled bool

		store := &providermock.Store{
			PutFunc: func(_ context.Context, _, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				putCalled = true

				assert.Equal(t, "v2", value)
				assert.Equal(t, domain.ValueTypePlaintext, vt)

				return domain.Version{}, nil
			},
		}
		s := staging.NewAzureAppConfigParamStrategy(store)

		err := s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v2")})
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
		s := staging.NewAzureAppConfigParamStrategy(store)

		err := s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationDelete})
		require.NoError(t, err)
		assert.Equal(t, "cfg", deleted)
	})

	t.Run("delete already-gone is success", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
				return secretNotFound(name)
			},
		}
		s := staging.NewAzureAppConfigParamStrategy(store)

		require.NoError(t, s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationDelete}))
	})

	t.Run("unknown operation errors", func(t *testing.T) {
		t.Parallel()

		s := staging.NewAzureAppConfigParamStrategy(&providermock.Store{})
		err := s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.Operation("bogus")})
		require.Error(t, err)
	})
}

func TestAzureAppConfigParamStrategy_ErrorWrapping(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	t.Run("create error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				return domain.Version{}, boom
			},
		}
		s := staging.NewAzureAppConfigParamStrategy(store)
		err := s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationCreate, Value: lo.ToPtr("v")})
		require.ErrorIs(t, err, boom)
	})

	t.Run("update with nil value is a no-op", func(t *testing.T) {
		t.Parallel()

		s := staging.NewAzureAppConfigParamStrategy(&providermock.Store{})
		require.NoError(t, s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationUpdate}))
	})

	t.Run("update error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				return domain.Version{}, boom
			},
		}
		s := staging.NewAzureAppConfigParamStrategy(store)
		err := s.Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v")})
		require.ErrorIs(t, err, boom)
	})

	t.Run("delete error (non-not-found) is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error { return boom },
		}
		err := staging.NewAzureAppConfigParamStrategy(store).Apply(t.Context(), "cfg", staging.Entry{Operation: staging.OperationDelete})
		require.ErrorIs(t, err, boom)
	})
}

// TestAzureAppConfigParamStrategy_LastWriteWins asserts the last-write-wins
// semantics: FetchLastModified always returns zero time and never touches the
// store, so no modified-after conflict is ever reported.
func TestAzureAppConfigParamStrategy_LastWriteWins(t *testing.T) {
	t.Parallel()

	// A store whose funcs would panic if called: FetchLastModified must not
	// call the store at all.
	got, err := staging.NewAzureAppConfigParamStrategy(&providermock.Store{}).FetchLastModified(t.Context(), "cfg")
	require.NoError(t, err)
	assert.True(t, got.IsZero())
}

// TestAzureAppConfigParamStrategy_ApplyTags asserts staged tag changes forward
// to the store's Tag (adds) and Untag (removes).
func TestAzureAppConfigParamStrategy_ApplyTags(t *testing.T) {
	t.Parallel()

	t.Run("adds and removes forward to Tag/Untag", func(t *testing.T) {
		t.Parallel()

		var (
			gotAdd    map[string]string
			gotRemove []string
		)

		store := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, add map[string]string) error {
				gotAdd = add

				return nil
			},
			UntagFunc: func(_ context.Context, _ string, keys []string) error {
				gotRemove = keys

				return nil
			},
		}
		s := staging.NewAzureAppConfigParamStrategy(store)

		err := s.ApplyTags(t.Context(), "cfg", staging.TagEntry{
			Add:    map[string]string{"env": "prod"},
			Remove: maputil.NewSet("old"),
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod"}, gotAdd)
		assert.Equal(t, []string{"old"}, gotRemove)
	})

	t.Run("add error is wrapped", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		store := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, _ map[string]string) error { return boom },
		}
		s := staging.NewAzureAppConfigParamStrategy(store)
		err := s.ApplyTags(t.Context(), "cfg", staging.TagEntry{Add: map[string]string{"k": "v"}})
		require.ErrorIs(t, err, boom)
	})

	t.Run("remove error is wrapped", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		store := &providermock.Store{
			UntagFunc: func(_ context.Context, _ string, _ []string) error { return boom },
		}
		s := staging.NewAzureAppConfigParamStrategy(store)
		err := s.ApplyTags(t.Context(), "cfg", staging.TagEntry{Remove: maputil.NewSet("k")})
		require.ErrorIs(t, err, boom)
	})
}

func TestAzureAppConfigParamStrategy_Fetch(t *testing.T) {
	t.Parallel()

	t.Run("FetchCurrent returns value with empty identifier", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Name: name, Value: "current"}, nil
			},
		}
		fr, err := staging.NewAzureAppConfigParamStrategy(store).FetchCurrent(t.Context(), "cfg")
		require.NoError(t, err)
		assert.Equal(t, "current", fr.Value)
		assert.Empty(t, fr.Identifier)
	})

	t.Run("FetchCurrent propagates error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, err := staging.NewAzureAppConfigParamStrategy(store).FetchCurrent(t.Context(), "cfg")
		require.ErrorIs(t, err, boom)
	})

	t.Run("FetchCurrentTags returns the setting's tags", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{
					Name: name,
					Tags: []domain.Tag{{Key: "env", Value: "prod"}, {Key: "team", Value: "core"}},
				}, nil
			},
		}
		tags, err := staging.NewAzureAppConfigParamStrategy(store).FetchCurrentTags(t.Context(), "cfg")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "team": "core"}, tags)
	})

	t.Run("FetchCurrentTags: not-found yields nil, no tags yields nil", func(t *testing.T) {
		t.Parallel()

		notFound := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound(name)
			},
		}
		tags, err := staging.NewAzureAppConfigParamStrategy(notFound).FetchCurrentTags(t.Context(), "cfg")
		require.NoError(t, err)
		assert.Nil(t, tags)

		noTags := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Name: name}, nil
			},
		}
		tags, err = staging.NewAzureAppConfigParamStrategy(noTags).FetchCurrentTags(t.Context(), "cfg")
		require.NoError(t, err)
		assert.Nil(t, tags)
	})

	t.Run("FetchCurrentValue returns value with zero LastModified", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Name: name, Value: "current"}, nil
			},
		}
		result, err := staging.NewAzureAppConfigParamStrategy(store).FetchCurrentValue(t.Context(), "cfg")
		require.NoError(t, err)
		assert.Equal(t, "current", result.Value)
		assert.True(t, result.LastModified.IsZero())
	})

	t.Run("FetchCurrentValue: not-found maps to ResourceNotFoundError, other errors pass through", func(t *testing.T) {
		t.Parallel()

		notFound := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound(name)
			},
		}
		_, err := staging.NewAzureAppConfigParamStrategy(notFound).FetchCurrentValue(t.Context(), "cfg")

		var rnf *staging.ResourceNotFoundError

		require.ErrorAs(t, err, &rnf)

		boom := errors.New("boom")
		errStore := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, err = staging.NewAzureAppConfigParamStrategy(errStore).FetchCurrentValue(t.Context(), "cfg")
		require.ErrorIs(t, err, boom)
	})
}

func TestAzureAppConfigParamStrategy_ParseAndVersion(t *testing.T) {
	t.Parallel()

	s := staging.NewAzureAppConfigParamStrategy(&providermock.Store{})

	t.Run("ParseName: whole argument is the key, including specifier-like chars", func(t *testing.T) {
		t.Parallel()

		name, err := s.ParseName("cfg")
		require.NoError(t, err)
		assert.Equal(t, "cfg", name)

		// ':' / '#' / '~' are legal App Configuration key characters (#353).
		for _, key := range []string{"cfg#1", "cfg~1", "cfg:lbl", "Logging:LogLevel:Default"} {
			got, err := s.ParseName(key)
			require.NoError(t, err, key)
			assert.Equal(t, key, got)
		}
	})

	t.Run("ParseSpec: whole argument is the key, never has a version", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("cfg")
		require.NoError(t, err)
		assert.Equal(t, "cfg", name)
		assert.False(t, hasVersion)

		name, hasVersion, err = s.ParseSpec("cfg#1")
		require.NoError(t, err)
		assert.Equal(t, "cfg#1", name)
		assert.False(t, hasVersion)
	})

	t.Run("FetchVersion resolves current for a bare name", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
				assert.True(t, ref.IsLatest())

				return &domain.Entry{Value: "current"}, nil
			},
		}
		value, label, err := staging.NewAzureAppConfigParamStrategy(store).FetchVersion(t.Context(), "cfg")
		require.NoError(t, err)
		assert.Equal(t, "current", value)
		assert.Equal(t, "current", label)
	})

	t.Run("FetchVersion resolves a specifier-like key as the whole key", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
				assert.Equal(t, "cfg#1", name)
				assert.True(t, ref.IsLatest())

				return &domain.Entry{Value: "current"}, nil
			},
		}
		value, label, err := staging.NewAzureAppConfigParamStrategy(store).FetchVersion(t.Context(), "cfg#1")
		require.NoError(t, err)
		assert.Equal(t, "current", value)
		assert.Equal(t, "current", label)
	})

	t.Run("FetchVersion propagates get error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) { return nil, boom },
		}
		_, _, err := staging.NewAzureAppConfigParamStrategy(store).FetchVersion(t.Context(), "cfg")
		require.ErrorIs(t, err, boom)
	})
}

func TestAzureAppConfigParamParserFactory(t *testing.T) {
	t.Parallel()

	p := staging.AzureAppConfigParamParserFactory()
	assert.Equal(t, staging.ServiceParam, p.Service())
	assert.False(t, p.HasDeleteOptions())
}
