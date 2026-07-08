package staging_test

import (
	"context"
	"errors"
	"fmt"
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

func paramNotFound(name string) error {
	return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
}

func TestParamStrategy_BasicMethods(t *testing.T) {
	t.Parallel()

	s := staging.NewParamStrategy(nil)

	t.Run("Service", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, staging.ServiceParam, s.Service())
	})

	t.Run("ServiceName", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "SSM Parameter Store", s.ServiceName())
	})

	t.Run("ItemName", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "parameter", s.ItemName())
	})

	t.Run("HasDeleteOptions", func(t *testing.T) {
		t.Parallel()
		assert.False(t, s.HasDeleteOptions())
	})
}

func TestParamStrategy_Apply(t *testing.T) {
	t.Parallel()

	t.Run("create operation - new parameter", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			CreateFunc: func(
				_ context.Context, name, value string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				// Create is create-only; new parameters are plain String.
				assert.Equal(t, "/app/param", name)
				assert.Equal(t, "new-value", value)
				assert.Equal(t, domain.ValueTypePlaintext, valueType)

				return domain.Version{ID: "1"}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
		})
		require.NoError(t, err)
	})

	t.Run("update operation - preserves type", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "old-value", Type: domain.ValueTypeSecret}, nil
			},
			PutFunc: func(
				_ context.Context, _, _ string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				// Update preserves the existing (SecureString/secret) type and overwrites.
				assert.Equal(t, domain.ValueTypeSecret, valueType)

				return domain.Version{ID: "2"}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.NoError(t, err)
	})

	t.Run("delete operation", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
				assert.Equal(t, "/app/param", name)

				return nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)
	})

	t.Run("unknown operation", func(t *testing.T) {
		t.Parallel()

		s := staging.NewParamStrategy(&providermock.Store{})
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.Operation("unknown"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation")
	})

	t.Run("update get parameter error (not ErrNotFound)", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, errors.New("access denied")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get existing parameter")
	})

	t.Run("update parameter not found error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, paramNotFound("/app/param")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parameter not found")
	})

	t.Run("create put parameter error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			CreateFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				return domain.Version{}, errors.New("put failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create parameter")
	})

	t.Run("update put parameter error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "old-value"}, nil
			},
			PutFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				return domain.Version{}, errors.New("put failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update parameter")
	})

	t.Run("delete parameter error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
				return errors.New("delete failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete parameter")
	})
}

func TestParamStrategy_FetchCurrent(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "current-value", Version: domain.Version{ID: "5"}}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchCurrent(t.Context(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, "current-value", result.Value)
		assert.Equal(t, "#5", result.Identifier)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, errors.New("not found")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchCurrent(t.Context(), "/app/param")
		require.Error(t, err)
	})
}

func TestParamStrategy_ParseName(t *testing.T) {
	t.Parallel()

	s := staging.NewParamStrategy(nil)

	t.Run("valid name", func(t *testing.T) {
		t.Parallel()

		name, err := s.ParseName("/app/param")
		require.NoError(t, err)
		assert.Equal(t, "/app/param", name)
	})

	t.Run("name with version", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("/app/param#5")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain a version specifier")
	})

	t.Run("name with shift", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("/app/param~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain a version specifier")
	})

	t.Run("name is valid even without slash prefix", func(t *testing.T) {
		t.Parallel()

		name, err := s.ParseName("myParam")
		require.NoError(t, err)
		assert.Equal(t, "myParam", name)
	})

	t.Run("parse error - invalid version format", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("/app/param#abc")
		require.Error(t, err)
	})
}

func TestParamStrategy_FetchCurrentValue(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "fetched-value", Modified: &now}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchCurrentValue(t.Context(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, "fetched-value", result.Value)
		assert.Equal(t, now, result.LastModified)
	})

	t.Run("not found returns ResourceNotFoundError", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, paramNotFound("/app/param")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchCurrentValue(t.Context(), "/app/param")
		require.Error(t, err)

		var notFound *staging.ResourceNotFoundError
		assert.ErrorAs(t, err, &notFound)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchCurrentValue(t.Context(), "/app/param")
		require.Error(t, err)
	})
}

func TestParamStrategy_ParseSpec(t *testing.T) {
	t.Parallel()

	s := staging.NewParamStrategy(nil)

	t.Run("name only", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("/app/param")
		require.NoError(t, err)
		assert.Equal(t, "/app/param", name)
		assert.False(t, hasVersion)
	})

	t.Run("with version", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("/app/param#5")
		require.NoError(t, err)
		assert.Equal(t, "/app/param", name)
		assert.True(t, hasVersion)
	})

	t.Run("with shift", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("/app/param~1")
		require.NoError(t, err)
		assert.Equal(t, "/app/param", name)
		assert.True(t, hasVersion)
	})

	t.Run("invalid spec - bad version", func(t *testing.T) {
		t.Parallel()

		_, _, err := s.ParseSpec("/app/param#abc")
		require.Error(t, err)
	})
}

func TestParamStrategy_FetchVersion(t *testing.T) {
	t.Parallel()

	t.Run("success with version", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			ResolveFunc: func(_ context.Context, name, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "/app/param", name)
				assert.Equal(t, "#2", spec)

				return provider.NewVersionRef("2"), nil
			},
			GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
				assert.Equal(t, "2", ref.ID())

				return &domain.Entry{Value: "v2", Version: domain.Version{ID: "2"}}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		value, label, err := s.FetchVersion(t.Context(), "/app/param#2")
		require.NoError(t, err)
		assert.Equal(t, "v2", value)
		assert.Equal(t, "#2", label)
	})

	t.Run("success with shift", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			ResolveFunc: func(_ context.Context, name, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "/app/param", name)
				assert.Equal(t, "~1", spec)

				return provider.NewVersionRef("2"), nil
			},
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "v2", Version: domain.Version{ID: "2"}}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		value, label, err := s.FetchVersion(t.Context(), "/app/param~1")
		require.NoError(t, err)
		assert.Equal(t, "v2", value)
		assert.Equal(t, "#2", label)
	})

	t.Run("parse error - invalid version format", func(t *testing.T) {
		t.Parallel()

		s := staging.NewParamStrategy(&providermock.Store{})
		_, _, err := s.FetchVersion(t.Context(), "/app/param#abc")
		require.Error(t, err)
	})

	t.Run("fetch error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.VersionRef{}, errors.New("fetch error")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, _, err := s.FetchVersion(t.Context(), "/app/param#2")
		require.Error(t, err)
	})
}

func TestParamParserFactory(t *testing.T) {
	t.Parallel()

	parser := staging.ParamParserFactory()
	require.NotNil(t, parser)
	assert.Equal(t, staging.ServiceParam, parser.Service())
}

func TestParamStrategy_FetchLastModified(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("success - returns last modified time", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Modified: &now}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchLastModified(t.Context(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, now, result)
	})

	t.Run("not found - returns ResourceNotFoundError", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, paramNotFound("/app/param")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchLastModified(t.Context(), "/app/param")
		notFoundErr := (*staging.ResourceNotFoundError)(nil)
		require.ErrorAs(t, err, &notFoundErr)
		require.ErrorIs(t, err, provider.ErrNotFound)
	})

	t.Run("other error - returns error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, errors.New("access denied")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchLastModified(t.Context(), "/app/param")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get parameter")
	})

	t.Run("nil modified time - returns zero time", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Modified: nil}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchLastModified(t.Context(), "/app/param")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})
}

func TestParamStrategy_Apply_WithDescription(t *testing.T) {
	t.Parallel()

	t.Run("create with description", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			CreateFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				assert.Equal(t, "Test description", description)

				return domain.Version{ID: "1"}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation:   staging.OperationCreate,
			Value:       lo.ToPtr("value"),
			Description: lo.ToPtr("Test description"),
		})
		require.NoError(t, err)
	})

	t.Run("update with description", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "old-value"}, nil
			},
			PutFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				assert.Equal(t, "Test description", description)

				return domain.Version{ID: "2"}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(t.Context(), "/app/param", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       lo.ToPtr("value"),
			Description: lo.ToPtr("Test description"),
		})
		require.NoError(t, err)
	})
}

func TestParamStrategy_Apply_DeleteAlreadyDeleted(t *testing.T) {
	t.Parallel()

	mock := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
			return paramNotFound(name)
		},
	}

	s := staging.NewParamStrategy(mock)
	err := s.Apply(t.Context(), "/app/param", staging.Entry{
		Operation: staging.OperationDelete,
	})
	require.NoError(t, err) // Should succeed even if already deleted
}

func TestParamStrategy_ApplyTags(t *testing.T) {
	t.Parallel()

	t.Run("add tags", func(t *testing.T) {
		t.Parallel()

		addTagsCalled := false
		mock := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, add map[string]string) error {
				addTagsCalled = true

				assert.Len(t, add, 1)

				return nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.ApplyTags(t.Context(), "/app/param", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)
		assert.True(t, addTagsCalled)
	})

	t.Run("remove tags", func(t *testing.T) {
		t.Parallel()

		removeTagsCalled := false
		mock := &providermock.Store{
			UntagFunc: func(_ context.Context, _ string, keys []string) error {
				removeTagsCalled = true

				assert.Contains(t, keys, "old-tag")

				return nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.ApplyTags(t.Context(), "/app/param", staging.TagEntry{
			Remove: maputil.NewSet("old-tag"),
		})
		require.NoError(t, err)
		assert.True(t, removeTagsCalled)
	})

	t.Run("add and remove tags", func(t *testing.T) {
		t.Parallel()

		addTagsCalled := false
		removeTagsCalled := false
		mock := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, add map[string]string) error {
				addTagsCalled = true

				assert.Len(t, add, 1)

				return nil
			},
			UntagFunc: func(_ context.Context, _ string, keys []string) error {
				removeTagsCalled = true

				assert.Contains(t, keys, "deprecated")

				return nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.ApplyTags(t.Context(), "/app/param", staging.TagEntry{
			Add:    map[string]string{"env": "prod"},
			Remove: maputil.NewSet("deprecated"),
		})
		require.NoError(t, err)
		assert.True(t, addTagsCalled)
		assert.True(t, removeTagsCalled)
	})

	t.Run("add tags error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
				return errors.New("tagging failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.ApplyTags(t.Context(), "/app/param", staging.TagEntry{
			Add: map[string]string{"env": "test"},
		})
		require.Error(t, err)
	})

	t.Run("remove tags error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			UntagFunc: func(_ context.Context, _ string, _ []string) error {
				return errors.New("untagging failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.ApplyTags(t.Context(), "/app/param", staging.TagEntry{
			Remove: maputil.NewSet("old-tag"),
		})
		require.Error(t, err)
	})
}

func TestParamStrategy_FetchCurrentValue_NoLastModified(t *testing.T) {
	t.Parallel()

	mock := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "value", Modified: nil}, nil
		},
	}

	s := staging.NewParamStrategy(mock)
	result, err := s.FetchCurrentValue(t.Context(), "/app/param")
	require.NoError(t, err)
	assert.Equal(t, "value", result.Value)
	assert.True(t, result.LastModified.IsZero())
}

func TestParamStrategy_FetchCurrentTags(t *testing.T) {
	t.Parallel()

	t.Run("returns tags successfully", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				assert.Equal(t, "/app/param", name)

				return &domain.Entry{Tags: []domain.Tag{
					{Key: "env", Value: "prod"},
					{Key: "team", Value: "backend"},
				}}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "team": "backend"}, tags)
	})

	t.Run("returns nil when parameter not found", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, paramNotFound("/app/nonexistent")
			},
		}

		s := staging.NewParamStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "/app/nonexistent")
		require.NoError(t, err)
		assert.Nil(t, tags)
	})

	t.Run("returns error for other API errors", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, errors.New("API error")
			},
		}

		s := staging.NewParamStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "/app/param")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get tags")
		assert.Nil(t, tags)
	})

	t.Run("returns nil when no tags exist", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Tags: nil}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "/app/param")
		require.NoError(t, err)
		assert.Nil(t, tags)
	})
}
