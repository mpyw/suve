package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/staging"
)

type secretMockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretapi.ListSecretVersionIdsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error)
	createSecretFunc         func(ctx context.Context, params *secretapi.CreateSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error)
	putSecretValueFunc       func(ctx context.Context, params *secretapi.PutSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error)
	deleteSecretFunc         func(ctx context.Context, params *secretapi.DeleteSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error)
	updateSecretFunc         func(ctx context.Context, params *secretapi.UpdateSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error)
	tagResourceFunc          func(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error)
	untagResourceFunc        func(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error)
}

func (m *secretMockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, errors.New("GetSecretValue not mocked")
}

func (m *secretMockClient) ListSecretVersionIds(ctx context.Context, params *secretapi.ListSecretVersionIdsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, errors.New("ListSecretVersionIds not mocked")
}

func (m *secretMockClient) CreateSecret(ctx context.Context, params *secretapi.CreateSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error) {
	if m.createSecretFunc != nil {
		return m.createSecretFunc(ctx, params, optFns...)
	}
	return nil, errors.New("CreateSecret not mocked")
}

func (m *secretMockClient) PutSecretValue(ctx context.Context, params *secretapi.PutSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}
	return nil, errors.New("PutSecretValue not mocked")
}

func (m *secretMockClient) DeleteSecret(ctx context.Context, params *secretapi.DeleteSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
	if m.deleteSecretFunc != nil {
		return m.deleteSecretFunc(ctx, params, optFns...)
	}
	return nil, errors.New("DeleteSecret not mocked")
}

func (m *secretMockClient) UpdateSecret(ctx context.Context, params *secretapi.UpdateSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
	if m.updateSecretFunc != nil {
		return m.updateSecretFunc(ctx, params, optFns...)
	}
	return &secretapi.UpdateSecretOutput{}, nil
}

func (m *secretMockClient) TagResource(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
	if m.tagResourceFunc != nil {
		return m.tagResourceFunc(ctx, params, optFns...)
	}
	return &secretapi.TagResourceOutput{}, nil
}

func (m *secretMockClient) UntagResource(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
	if m.untagResourceFunc != nil {
		return m.untagResourceFunc(ctx, params, optFns...)
	}
	return &secretapi.UntagResourceOutput{}, nil
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
		mock := &secretMockClient{
			createSecretFunc: func(_ context.Context, params *secretapi.CreateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error) {
				assert.Equal(t, "my-secret", lo.FromPtr(params.Name))
				assert.Equal(t, "secret-value", lo.FromPtr(params.SecretString))
				return &secretapi.CreateSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		})
		require.NoError(t, err)
	})

	t.Run("create operation error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			createSecretFunc: func(_ context.Context, _ *secretapi.CreateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error) {
				return nil, errors.New("create failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create secret")
	})

	t.Run("update operation", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, params *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				assert.Equal(t, "updated-value", lo.FromPtr(params.SecretString))
				return &secretapi.PutSecretValueOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.NoError(t, err)
	})

	t.Run("update operation error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				return nil, errors.New("update failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update secret")
	})

	t.Run("delete operation - basic", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, params *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				assert.Nil(t, params.ForceDeleteWithoutRecovery)
				assert.Nil(t, params.RecoveryWindowInDays)
				return &secretapi.DeleteSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)
	})

	t.Run("delete operation - with force", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, params *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
				assert.True(t, lo.FromPtr(params.ForceDeleteWithoutRecovery))
				return &secretapi.DeleteSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
			DeleteOptions: &staging.DeleteOptions{
				Force: true,
			},
		})
		require.NoError(t, err)
	})

	t.Run("delete operation - with recovery window", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, params *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
				assert.Nil(t, params.ForceDeleteWithoutRecovery)
				assert.Equal(t, int64(14), lo.FromPtr(params.RecoveryWindowInDays))
				return &secretapi.DeleteSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
			DeleteOptions: &staging.DeleteOptions{
				RecoveryWindow: 14,
			},
		})
		require.NoError(t, err)
	})

	t.Run("delete operation error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, _ *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
				return nil, errors.New("delete failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete secret")
	})

	t.Run("unknown operation", func(t *testing.T) {
		t.Parallel()
		s := staging.NewSecretStrategy(&secretMockClient{})
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
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
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return &secretapi.GetSecretValueOutput{
					SecretString: lo.ToPtr("secret-value"),
					VersionId:    lo.ToPtr("abcdefgh-1234-5678-9abc-def012345678"),
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchCurrent(context.Background(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, "secret-value", result.Value)
		assert.Equal(t, "#abcdefgh", result.Identifier)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return nil, errors.New("not found")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchCurrent(context.Background(), "my-secret")
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
		assert.Contains(t, err.Error(), "without version specifier")
	})

	t.Run("name with label", func(t *testing.T) {
		t.Parallel()
		_, err := s.ParseName("my-secret:AWSCURRENT")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without version specifier")
	})

	t.Run("name with shift", func(t *testing.T) {
		t.Parallel()
		_, err := s.ParseName("my-secret~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without version specifier")
	})
}

func TestSecretStrategy_FetchCurrentValue(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return &secretapi.GetSecretValueOutput{
					SecretString: lo.ToPtr("fetched-secret"),
					CreatedDate:  &now,
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchCurrentValue(context.Background(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, "fetched-secret", result.Value)
		assert.Equal(t, now, result.LastModified)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchCurrentValue(context.Background(), "my-secret")
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
}

func TestSecretStrategy_FetchVersion(t *testing.T) {
	t.Parallel()

	t.Run("success with label", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				assert.Equal(t, "AWSPREVIOUS", lo.FromPtr(params.VersionStage))
				return &secretapi.GetSecretValueOutput{
					SecretString: lo.ToPtr("previous-value"),
					VersionId:    lo.ToPtr("12345678-abcd-efgh-ijkl-mnopqrstuvwx"),
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		value, label, err := s.FetchVersion(context.Background(), "my-secret:AWSPREVIOUS")
		require.NoError(t, err)
		assert.Equal(t, "previous-value", value)
		assert.Equal(t, "#12345678", label)
	})

	t.Run("success with shift", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				if params.VersionStage != nil && lo.FromPtr(params.VersionStage) == "AWSCURRENT" {
					return &secretapi.GetSecretValueOutput{
						SecretString: lo.ToPtr("current-value"),
						VersionId:    lo.ToPtr("version-current"),
					}, nil
				}
				return &secretapi.GetSecretValueOutput{
					SecretString: lo.ToPtr("shifted-value"),
					VersionId:    lo.ToPtr("version-shifted"),
				}, nil
			},
			listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIdsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error) {
				return &secretapi.ListSecretVersionIdsOutput{
					Versions: []secretapi.SecretVersionsListEntry{
						{VersionId: lo.ToPtr("version-current"), VersionStages: []string{"AWSCURRENT"}},
						{VersionId: lo.ToPtr("version-shifted"), VersionStages: []string{"AWSPREVIOUS"}},
					},
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		value, label, err := s.FetchVersion(context.Background(), "my-secret~1")
		require.NoError(t, err)
		assert.Equal(t, "shifted-value", value)
		assert.Equal(t, "#version-", label)
	})

	t.Run("parse error", func(t *testing.T) {
		t.Parallel()
		s := staging.NewSecretStrategy(&secretMockClient{})
		_, _, err := s.FetchVersion(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("fetch error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, _, err := s.FetchVersion(context.Background(), "my-secret:AWSCURRENT")
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

	t.Run("success - returns created date", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return &secretapi.GetSecretValueOutput{
					Name:        lo.ToPtr("my-secret"),
					CreatedDate: &now,
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, now, result)
	})

	t.Run("not found - returns zero time", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return nil, &secretapi.ResourceNotFoundException{}
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "my-secret")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})

	t.Run("other error - returns error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return nil, errors.New("access denied")
			},
		}

		s := staging.NewSecretStrategy(mock)
		_, err := s.FetchLastModified(context.Background(), "my-secret")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get secret")
	})

	t.Run("nil CreatedDate - returns zero time", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
				return &secretapi.GetSecretValueOutput{
					Name:        lo.ToPtr("my-secret"),
					CreatedDate: nil,
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "my-secret")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})
}

func TestSecretStrategy_Apply_WithOptions(t *testing.T) {
	t.Parallel()

	t.Run("create with description", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			createSecretFunc: func(_ context.Context, params *secretapi.CreateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error) {
				assert.Equal(t, "Test description", lo.FromPtr(params.Description))
				return &secretapi.CreateSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation:   staging.OperationCreate,
			Value:       lo.ToPtr("secret-value"),
			Description: lo.ToPtr("Test description"),
		})
		require.NoError(t, err)
	})

	t.Run("create with tags", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			createSecretFunc: func(_ context.Context, params *secretapi.CreateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error) {
				assert.Len(t, params.Tags, 2)
				return &secretapi.CreateSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
			Tags:      map[string]string{"env": "test", "owner": "team"},
		})
		require.NoError(t, err)
	})

	t.Run("update with description", func(t *testing.T) {
		t.Parallel()
		updateSecretCalled := false
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				return &secretapi.PutSecretValueOutput{}, nil
			},
			updateSecretFunc: func(_ context.Context, params *secretapi.UpdateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
				updateSecretCalled = true
				assert.Equal(t, "Updated description", lo.FromPtr(params.Description))
				return &secretapi.UpdateSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       lo.ToPtr("updated-value"),
			Description: lo.ToPtr("Updated description"),
		})
		require.NoError(t, err)
		assert.True(t, updateSecretCalled)
	})

	t.Run("update with description error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				return &secretapi.PutSecretValueOutput{}, nil
			},
			updateSecretFunc: func(_ context.Context, _ *secretapi.UpdateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
				return nil, errors.New("update description failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       lo.ToPtr("updated-value"),
			Description: lo.ToPtr("Updated description"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update description")
	})

	t.Run("update with tags", func(t *testing.T) {
		t.Parallel()
		tagResourceCalled := false
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				return &secretapi.PutSecretValueOutput{}, nil
			},
			tagResourceFunc: func(_ context.Context, params *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
				tagResourceCalled = true
				assert.Len(t, params.Tags, 1)
				return &secretapi.TagResourceOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
			Tags:      map[string]string{"env": "prod"},
		})
		require.NoError(t, err)
		assert.True(t, tagResourceCalled)
	})

	t.Run("update with untag keys", func(t *testing.T) {
		t.Parallel()
		untagResourceCalled := false
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				return &secretapi.PutSecretValueOutput{}, nil
			},
			untagResourceFunc: func(_ context.Context, params *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
				untagResourceCalled = true
				assert.Contains(t, params.TagKeys, "old-tag")
				return &secretapi.UntagResourceOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
			UntagKeys: []string{"old-tag"},
		})
		require.NoError(t, err)
		assert.True(t, untagResourceCalled)
	})

	t.Run("update tagging error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				return &secretapi.PutSecretValueOutput{}, nil
			},
			tagResourceFunc: func(_ context.Context, _ *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
				return nil, errors.New("tagging failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
			Tags:      map[string]string{"env": "prod"},
		})
		require.Error(t, err)
	})

	t.Run("delete already deleted", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, _ *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
				return nil, &secretapi.ResourceNotFoundException{}
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err) // Should succeed even if already deleted
	})
}

func TestSecretStrategy_FetchCurrentValue_NoCreatedDate(t *testing.T) {
	t.Parallel()

	mock := &secretMockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				SecretString: lo.ToPtr("secret-value"),
				CreatedDate:  nil,
			}, nil
		},
	}

	s := staging.NewSecretStrategy(mock)
	result, err := s.FetchCurrentValue(context.Background(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", result.Value)
	assert.True(t, result.LastModified.IsZero())
}

func TestSecretStrategy_Apply_TagOnlyUpdate(t *testing.T) {
	t.Parallel()

	t.Run("tag-only update without value change", func(t *testing.T) {
		t.Parallel()
		tagResourceCalled := false
		putSecretValueCalled := false

		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				putSecretValueCalled = true
				return &secretapi.PutSecretValueOutput{}, nil
			},
			tagResourceFunc: func(_ context.Context, params *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
				tagResourceCalled = true
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				assert.Len(t, params.Tags, 1)
				return &secretapi.TagResourceOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     nil, // No value change
			Tags:      map[string]string{"env": "prod"},
		})
		require.NoError(t, err)
		assert.True(t, tagResourceCalled, "TagResource should be called")
		assert.False(t, putSecretValueCalled, "PutSecretValue should NOT be called when Value is nil")
	})

	t.Run("untag-only update without value change", func(t *testing.T) {
		t.Parallel()
		untagResourceCalled := false
		putSecretValueCalled := false

		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
				putSecretValueCalled = true
				return &secretapi.PutSecretValueOutput{}, nil
			},
			untagResourceFunc: func(_ context.Context, params *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
				untagResourceCalled = true
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				assert.Equal(t, []string{"old-tag"}, params.TagKeys)
				return &secretapi.UntagResourceOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     nil, // No value change
			UntagKeys: []string{"old-tag"},
		})
		require.NoError(t, err)
		assert.True(t, untagResourceCalled, "UntagResource should be called")
		assert.False(t, putSecretValueCalled, "PutSecretValue should NOT be called when Value is nil")
	})
}

func TestSecretStrategy_Apply_DeleteIgnoresTags(t *testing.T) {
	t.Parallel()

	t.Run("delete ignores tags and untag keys", func(t *testing.T) {
		t.Parallel()
		deleteSecretCalled := false
		tagResourceCalled := false
		untagResourceCalled := false

		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, params *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
				deleteSecretCalled = true
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				return &secretapi.DeleteSecretOutput{}, nil
			},
			tagResourceFunc: func(_ context.Context, _ *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
				tagResourceCalled = true
				return &secretapi.TagResourceOutput{}, nil
			},
			untagResourceFunc: func(_ context.Context, _ *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
				untagResourceCalled = true
				return &secretapi.UntagResourceOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Apply(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
			Tags:      map[string]string{"env": "prod"}, // Should be ignored
			UntagKeys: []string{"old-tag"},              // Should be ignored
		})
		require.NoError(t, err)
		assert.True(t, deleteSecretCalled, "DeleteSecret should be called")
		assert.False(t, tagResourceCalled, "TagResource should NOT be called during delete")
		assert.False(t, untagResourceCalled, "UntagResource should NOT be called during delete")
	})
}
