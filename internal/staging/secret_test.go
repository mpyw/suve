package staging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

type secretMockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
	createSecretFunc         func(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	putSecretValueFunc       func(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	deleteSecretFunc         func(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	updateSecretFunc         func(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error)
	tagResourceFunc          func(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error)
	untagResourceFunc        func(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error)
}

func (m *secretMockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, errors.New("GetSecretValue not mocked")
}

func (m *secretMockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, errors.New("ListSecretVersionIds not mocked")
}

func (m *secretMockClient) CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	if m.createSecretFunc != nil {
		return m.createSecretFunc(ctx, params, optFns...)
	}
	return nil, errors.New("CreateSecret not mocked")
}

func (m *secretMockClient) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}
	return nil, errors.New("PutSecretValue not mocked")
}

func (m *secretMockClient) DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	if m.deleteSecretFunc != nil {
		return m.deleteSecretFunc(ctx, params, optFns...)
	}
	return nil, errors.New("DeleteSecret not mocked")
}

func (m *secretMockClient) UpdateSecret(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
	if m.updateSecretFunc != nil {
		return m.updateSecretFunc(ctx, params, optFns...)
	}
	return &secretsmanager.UpdateSecretOutput{}, nil
}

func (m *secretMockClient) TagResource(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
	if m.tagResourceFunc != nil {
		return m.tagResourceFunc(ctx, params, optFns...)
	}
	return &secretsmanager.TagResourceOutput{}, nil
}

func (m *secretMockClient) UntagResource(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
	if m.untagResourceFunc != nil {
		return m.untagResourceFunc(ctx, params, optFns...)
	}
	return &secretsmanager.UntagResourceOutput{}, nil
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
		assert.Equal(t, "SM", s.ServiceName())
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

func TestSecretStrategy_Push(t *testing.T) {
	t.Parallel()

	t.Run("create operation", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			createSecretFunc: func(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
				assert.Equal(t, "my-secret", lo.FromPtr(params.Name))
				assert.Equal(t, "secret-value", lo.FromPtr(params.SecretString))
				return &secretsmanager.CreateSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "secret-value",
		})
		require.NoError(t, err)
	})

	t.Run("create operation error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			createSecretFunc: func(_ context.Context, _ *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
				return nil, errors.New("create failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "secret-value",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create secret")
	})

	t.Run("update operation", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				assert.Equal(t, "updated-value", lo.FromPtr(params.SecretString))
				return &secretsmanager.PutSecretValueOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "updated-value",
		})
		require.NoError(t, err)
	})

	t.Run("update operation error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
				return nil, errors.New("update failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "updated-value",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update secret")
	})

	t.Run("delete operation - basic", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
				assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
				assert.Nil(t, params.ForceDeleteWithoutRecovery)
				assert.Nil(t, params.RecoveryWindowInDays)
				return &secretsmanager.DeleteSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)
	})

	t.Run("delete operation - with force", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
				assert.True(t, lo.FromPtr(params.ForceDeleteWithoutRecovery))
				return &secretsmanager.DeleteSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
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
			deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
				assert.Nil(t, params.ForceDeleteWithoutRecovery)
				assert.Equal(t, int64(14), lo.FromPtr(params.RecoveryWindowInDays))
				return &secretsmanager.DeleteSecretOutput{}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
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
			deleteSecretFunc: func(_ context.Context, _ *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
				return nil, errors.New("delete failed")
			},
		}

		s := staging.NewSecretStrategy(mock)
		err := s.Push(context.Background(), "my-secret", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete secret")
	})

	t.Run("unknown operation", func(t *testing.T) {
		t.Parallel()
		s := staging.NewSecretStrategy(&secretMockClient{})
		err := s.Push(context.Background(), "my-secret", staging.Entry{
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
			getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
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
			getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
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

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: lo.ToPtr("fetched-secret"),
				}, nil
			},
		}

		s := staging.NewSecretStrategy(mock)
		value, err := s.FetchCurrentValue(context.Background(), "my-secret")
		require.NoError(t, err)
		assert.Equal(t, "fetched-secret", value)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &secretMockClient{
			getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
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
			getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				assert.Equal(t, "AWSPREVIOUS", lo.FromPtr(params.VersionStage))
				return &secretsmanager.GetSecretValueOutput{
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
			getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				if params.VersionStage != nil && lo.FromPtr(params.VersionStage) == "AWSCURRENT" {
					return &secretsmanager.GetSecretValueOutput{
						SecretString: lo.ToPtr("current-value"),
						VersionId:    lo.ToPtr("version-current"),
					}, nil
				}
				return &secretsmanager.GetSecretValueOutput{
					SecretString: lo.ToPtr("shifted-value"),
					VersionId:    lo.ToPtr("version-shifted"),
				}, nil
			},
			listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []smtypes.SecretVersionsListEntry{
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
			getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
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
