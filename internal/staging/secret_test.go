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
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

func secretNotFound(name string) error {
	return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
}

func TestSecretStrategy_BasicMethods(t *testing.T) {
	t.Parallel()

	s := staging.NewSecretStrategy(nil)

	t.Run("Service", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, staging.ServiceSecret, s.Service())
	})

	t.Run("ServiceName", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Secrets Manager", s.ServiceName())
	})

	t.Run("ItemName", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "secret", s.ItemName())
	})

	t.Run("HasDeleteOptions", func(t *testing.T) {
		t.Parallel()
		assert.True(t, s.HasDeleteOptions())
	})
}

func TestSecretStrategy_Apply(t *testing.T) {
	t.Parallel()

	t.Run("create operation", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			CreateFunc: func(
				_ context.Context, name, value string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				assert.Equal(t, "my-secret", name)
				assert.Equal(t, "secret-value", value)
				assert.Equal(t, domain.ValueTypeSecret, valueType)

				return domain.Version{ID: "v1"}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		})
		require.NoError(t, err)
	})

	t.Run("create operation error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			CreateFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				return domain.Version{}, errors.New("create failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create secret")
	})

	t.Run("update operation", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			PutFunc: func(
				_ context.Context, name, value string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				assert.Equal(t, "my-secret", name)
				assert.Equal(t, "updated-value", value)
				assert.Equal(t, domain.ValueTypeSecret, valueType)

				return domain.Version{ID: "v2"}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.NoError(t, err)
	})

	t.Run("update operation error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			PutFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				return domain.Version{}, errors.New("update failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update secret")
	})

	t.Run("delete operation - basic", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, opts ...provider.DeleteOption) error {
				assert.Equal(t, "my-secret", name)
				assert.Empty(t, opts)

				return nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)
	})

	t.Run("delete operation - with force", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
				require.Len(t, opts, 1)
				assert.IsType(t, provider.ForceDelete{}, opts[0])

				return nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
			DeleteOptions: &staging.DeleteOptions{
				Force: true,
			},
		})
		require.NoError(t, err)
	})

	t.Run("delete operation - with recovery window", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
				require.Len(t, opts, 1)
				rw, ok := opts[0].(awssecret.RecoveryWindow)
				require.True(t, ok)
				assert.Equal(t, int64(14), rw.Days)

				return nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
			DeleteOptions: &staging.DeleteOptions{
				RecoveryWindow: 14,
			},
		})
		require.NoError(t, err)
	})

	t.Run("delete operation error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
				return errors.New("delete failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete secret")
	})

	t.Run("unknown operation", func(t *testing.T) {
		t.Parallel()

		s := staging.NewSecretStrategy(&providermock.Store{})
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.Operation("unknown"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation")
	})
}

func TestSecretStrategy_FetchCurrent(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{
					Value:   "secret-value",
					Version: domain.Version{ID: "abcdefgh-1234-5678-9abc-def012345678"},
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchCurrent(t.Context(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, "secret-value", result.Value)
		assert.Equal(t, "#abcdefgh", result.Identifier)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, errors.New("not found")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchCurrent(t.Context(), "my-secret")
		require.Error(t, err)
	})
}

func TestSecretStrategy_ParseName(t *testing.T) {
	t.Parallel()

	s := staging.NewSecretStrategy(nil)

	t.Run("valid name", func(t *testing.T) {
		t.Parallel()

		name, err := s.ParseName("my-secret")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", name)
	})

	t.Run("name with version ID", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("my-secret#abc123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain a version specifier")
	})

	t.Run("name with label", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("my-secret:AWSCURRENT")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain a version specifier")
	})

	t.Run("name with shift", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("my-secret~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain a version specifier")
	})

	t.Run("parse error - empty version ID", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("my-secret#")
		require.Error(t, err)
	})

	t.Run("parse error - empty label", func(t *testing.T) {
		t.Parallel()

		_, err := s.ParseName("my-secret:")
		require.Error(t, err)
	})
}

func TestSecretStrategy_FetchCurrentValue(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Value: "fetched-secret", Modified: &now}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchCurrentValue(t.Context(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, "fetched-secret", result.Value)
		assert.Equal(t, now, result.LastModified)
	})

	t.Run("not found returns ResourceNotFoundError", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound("my-secret")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchCurrentValue(t.Context(), "my-secret")
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

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchCurrentValue(t.Context(), "my-secret")
		require.Error(t, err)
	})
}

func TestSecretStrategy_ParseSpec(t *testing.T) {
	t.Parallel()

	s := staging.NewSecretStrategy(nil)

	t.Run("name only", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("my-secret")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", name)
		assert.False(t, hasVersion)
	})

	t.Run("with version ID", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("my-secret#abc123")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", name)
		assert.True(t, hasVersion)
	})

	t.Run("with label", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("my-secret:AWSPREVIOUS")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", name)
		assert.True(t, hasVersion)
	})

	t.Run("with shift", func(t *testing.T) {
		t.Parallel()

		name, hasVersion, err := s.ParseSpec("my-secret~1")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", name)
		assert.True(t, hasVersion)
	})

	t.Run("parse error - empty version ID", func(t *testing.T) {
		t.Parallel()

		_, _, err := s.ParseSpec("my-secret#")
		require.Error(t, err)
	})

	t.Run("parse error - empty label", func(t *testing.T) {
		t.Parallel()

		_, _, err := s.ParseSpec("my-secret:")
		require.Error(t, err)
	})
}

func TestSecretStrategy_FetchVersion(t *testing.T) {
	t.Parallel()

	t.Run("success with label", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			ResolveFunc: func(_ context.Context, name, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "my-secret", name)
				assert.Equal(t, ":AWSPREVIOUS", spec)

				return provider.NewVersionRef("12345678-abcd-efgh-ijkl-mnopqrstuvwx"), nil
			},
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{
					Value:   "previous-value",
					Version: domain.Version{ID: "12345678-abcd-efgh-ijkl-mnopqrstuvwx"},
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		value, label, err := s.FetchVersion(t.Context(), "my-secret:AWSPREVIOUS")
		require.NoError(t, err)
		assert.Equal(t, "previous-value", value)
		assert.Equal(t, "#12345678", label)
	})

	t.Run("success with shift", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			ResolveFunc: func(_ context.Context, name, spec string) (provider.VersionRef, error) {
				assert.Equal(t, "my-secret", name)
				assert.Equal(t, "~1", spec)

				return provider.NewVersionRef("version-shifted"), nil
			},
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{
					Value:   "shifted-value",
					Version: domain.Version{ID: "version-shifted"},
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		value, label, err := s.FetchVersion(t.Context(), "my-secret~1")
		require.NoError(t, err)
		assert.Equal(t, "shifted-value", value)
		assert.Equal(t, "#version-", label)
	})

	t.Run("parse error", func(t *testing.T) {
		t.Parallel()

		s := staging.NewSecretStrategy(&providermock.Store{})
		_, _, err := s.FetchVersion(t.Context(), "")
		require.Error(t, err)
	})

	t.Run("fetch error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.VersionRef{}, errors.New("fetch error")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, _, err := s.FetchVersion(t.Context(), "my-secret:AWSCURRENT")
		require.Error(t, err)
	})
}

func TestSecretParserFactory(t *testing.T) {
	t.Parallel()

	parser := staging.SecretParserFactory()
	require.NotNil(t, parser)
	assert.Equal(t, staging.ServiceSecret, parser.Service())
}

func TestSecretStrategy_FetchLastModified(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("success - returns modified time", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Modified: &now}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchLastModified(t.Context(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, now, result)
	})

	t.Run("not found - returns ResourceNotFoundError", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound("my-secret")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchLastModified(t.Context(), "my-secret")
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

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchLastModified(t.Context(), "my-secret")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get secret")
	})

	t.Run("nil modified time - returns zero time", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Modified: nil}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchLastModified(t.Context(), "my-secret")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})
}

func TestSecretStrategy_Apply_WithOptions(t *testing.T) {
	t.Parallel()

	t.Run("create with description", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			CreateFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				assert.Equal(t, "Test description", description)

				return domain.Version{ID: "v1"}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation:   staging.OperationCreate,
			Value:       lo.ToPtr("secret-value"),
			Description: lo.ToPtr("Test description"),
		})
		require.NoError(t, err)
	})

	t.Run("update with description", func(t *testing.T) {
		t.Parallel()

		putCalled := false
		mock := &providermock.Store{
			PutFunc: func(
				_ context.Context, _, value string, _ domain.ValueType, description string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				putCalled = true

				// Value and description are updated together via a single Put.
				assert.Equal(t, "updated-value", value)
				assert.Equal(t, "Updated description", description)

				return domain.Version{ID: "v2"}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       lo.ToPtr("updated-value"),
			Description: lo.ToPtr("Updated description"),
		})
		require.NoError(t, err)
		assert.True(t, putCalled)
	})

	t.Run("update with description error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			PutFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
			) (domain.Version, error) {
				return domain.Version{}, errors.New("update failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       lo.ToPtr("updated-value"),
			Description: lo.ToPtr("Updated description"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update secret")
	})

	t.Run("delete already deleted", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
				return secretNotFound(name)
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(t.Context(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err) // Should succeed even if already deleted
	})
}

func TestSecretStrategy_FetchCurrentValue_NoCreatedDate(t *testing.T) {
	t.Parallel()

	mock := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "secret-value", Modified: nil}, nil
		},
	}

	s := staging.NewSecretStrategy(mock)
	result, err := s.FetchCurrentValue(t.Context(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", result.Value)
	assert.True(t, result.LastModified.IsZero())
}

func TestSecretStrategy_ApplyTags(t *testing.T) {
	t.Parallel()

	t.Run("add tags", func(t *testing.T) {
		t.Parallel()

		tagResourceCalled := false
		mock := &providermock.Store{
			TagFunc: func(_ context.Context, name string, add map[string]string) error {
				tagResourceCalled = true

				assert.Equal(t, "my-secret", name)
				assert.Len(t, add, 1)

				return nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.ApplyTags(t.Context(), "my-secret", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)
		assert.True(t, tagResourceCalled)
	})

	t.Run("remove tags", func(t *testing.T) {
		t.Parallel()

		untagResourceCalled := false
		mock := &providermock.Store{
			UntagFunc: func(_ context.Context, name string, keys []string) error {
				untagResourceCalled = true

				assert.Equal(t, "my-secret", name)
				assert.Contains(t, keys, "old-tag")

				return nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.ApplyTags(t.Context(), "my-secret", staging.TagEntry{
			Remove: maputil.NewSet("old-tag"),
		})
		require.NoError(t, err)
		assert.True(t, untagResourceCalled)
	})

	t.Run("add and remove tags", func(t *testing.T) {
		t.Parallel()

		tagResourceCalled := false
		untagResourceCalled := false
		mock := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, add map[string]string) error {
				tagResourceCalled = true

				assert.Len(t, add, 1)

				return nil
			},
			UntagFunc: func(_ context.Context, _ string, keys []string) error {
				untagResourceCalled = true

				assert.Contains(t, keys, "deprecated")

				return nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.ApplyTags(t.Context(), "my-secret", staging.TagEntry{
			Add:    map[string]string{"env": "prod"},
			Remove: maputil.NewSet("deprecated"),
		})
		require.NoError(t, err)
		assert.True(t, tagResourceCalled)
		assert.True(t, untagResourceCalled)
	})

	t.Run("add tags error", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
				return errors.New("tagging failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.ApplyTags(t.Context(), "my-secret", staging.TagEntry{
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

		s := staging.NewSecretStrategy(mock)
		err := s.ApplyTags(t.Context(), "my-secret", staging.TagEntry{
			Remove: maputil.NewSet("old-tag"),
		})
		require.Error(t, err)
	})
}

func TestSecretStrategy_FetchCurrentTags(t *testing.T) {
	t.Parallel()

	t.Run("returns tags successfully", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				assert.Equal(t, "my-secret", name)

				return &domain.Entry{Tags: []domain.Tag{
					{Key: "env", Value: "prod"},
					{Key: "team", Value: "backend"},
				}}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "team": "backend"}, tags)
	})

	t.Run("returns nil when secret not found", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, secretNotFound("nonexistent-secret")
			},
		}

		s := staging.NewSecretStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "nonexistent-secret")
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

		s := staging.NewSecretStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "my-secret")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to describe secret")
		assert.Nil(t, tags)
	})

	t.Run("returns nil when no tags exist", func(t *testing.T) {
		t.Parallel()

		mock := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Tags: nil}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		tags, err := s.FetchCurrentTags(t.Context(), "my-secret")
		require.NoError(t, err)
		assert.Nil(t, tags)
	})
}
