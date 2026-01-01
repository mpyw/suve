package staging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

type ssmMockClient struct {
	getParameterFunc           func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc    func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
	putParameterFunc           func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	deleteParameterFunc        func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	addTagsToResourceFunc      func(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error)
	removeTagsFromResourceFunc func(ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error)
}

func (m *ssmMockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, errors.New("GetParameter not mocked")
}

func (m *ssmMockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, errors.New("GetParameterHistory not mocked")
}

func (m *ssmMockClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return nil, errors.New("PutParameter not mocked")
}

func (m *ssmMockClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return nil, errors.New("DeleteParameter not mocked")
}

func (m *ssmMockClient) AddTagsToResource(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error) {
	if m.addTagsToResourceFunc != nil {
		return m.addTagsToResourceFunc(ctx, params, optFns...)
	}
	return &ssm.AddTagsToResourceOutput{}, nil
}

func (m *ssmMockClient) RemoveTagsFromResource(ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsFromResourceFunc != nil {
		return m.removeTagsFromResourceFunc(ctx, params, optFns...)
	}
	return &ssm.RemoveTagsFromResourceOutput{}, nil
}

func TestSSMStrategy_BasicMethods(t *testing.T) {
	t.Parallel()

	s := staging.NewSSMStrategy(nil)

	t.Run("Service", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, staging.ServiceSSM, s.Service())
	})

	t.Run("ServiceName", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "SSM", s.ServiceName())
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

func TestSSMStrategy_Push(t *testing.T) {
	t.Parallel()

	t.Run("create operation - new parameter", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return nil, &types.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
				assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
				assert.Equal(t, "new-value", lo.FromPtr(params.Value))
				assert.Equal(t, types.ParameterTypeString, params.Type)
				return &ssm.PutParameterOutput{Version: 1}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "new-value",
		})
		require.NoError(t, err)
	})

	t.Run("update operation - preserves type", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Name:  lo.ToPtr("/app/param"),
						Value: lo.ToPtr("old-value"),
						Type:  types.ParameterTypeSecureString,
					},
				}, nil
			},
			putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
				assert.Equal(t, types.ParameterTypeSecureString, params.Type)
				return &ssm.PutParameterOutput{Version: 2}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "updated-value",
		})
		require.NoError(t, err)
	})

	t.Run("delete operation", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			deleteParameterFunc: func(_ context.Context, params *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
				assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
				return &ssm.DeleteParameterOutput{}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)
	})

	t.Run("unknown operation", func(t *testing.T) {
		t.Parallel()
		s := staging.NewSSMStrategy(&ssmMockClient{})
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.Operation("unknown"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation")
	})

	t.Run("get parameter error (not ParameterNotFound)", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return nil, errors.New("access denied")
			},
		}

		s := staging.NewSSMStrategy(mock)
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "value",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get existing parameter")
	})

	t.Run("put parameter error", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return nil, &types.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, _ *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
				return nil, errors.New("put failed")
			},
		}

		s := staging.NewSSMStrategy(mock)
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "value",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set parameter")
	})

	t.Run("delete parameter error", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
				return nil, errors.New("delete failed")
			},
		}

		s := staging.NewSSMStrategy(mock)
		err := s.Push(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete parameter")
	})
}

func TestSSMStrategy_FetchCurrent(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Name:    lo.ToPtr("/app/param"),
						Value:   lo.ToPtr("current-value"),
						Version: 5,
					},
				}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		result, err := s.FetchCurrent(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, "current-value", result.Value)
		assert.Equal(t, "#5", result.Identifier)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return nil, errors.New("not found")
			},
		}

		s := staging.NewSSMStrategy(mock)
		_, err := s.FetchCurrent(context.Background(), "/app/param")
		require.Error(t, err)
	})
}

func TestSSMStrategy_ParseName(t *testing.T) {
	t.Parallel()

	s := staging.NewSSMStrategy(nil)

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
		assert.Contains(t, err.Error(), "without version specifier")
	})

	t.Run("name with shift", func(t *testing.T) {
		t.Parallel()
		_, err := s.ParseName("/app/param~1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without version specifier")
	})

	t.Run("name is valid even without slash prefix", func(t *testing.T) {
		t.Parallel()
		name, err := s.ParseName("myParam")
		require.NoError(t, err)
		assert.Equal(t, "myParam", name)
	})
}

func TestSSMStrategy_FetchCurrentValue(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Value: lo.ToPtr("fetched-value"),
					},
				}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		value, err := s.FetchCurrentValue(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, "fetched-value", value)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewSSMStrategy(mock)
		_, err := s.FetchCurrentValue(context.Background(), "/app/param")
		require.Error(t, err)
	})
}

func TestSSMStrategy_ParseSpec(t *testing.T) {
	t.Parallel()

	s := staging.NewSSMStrategy(nil)

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

func TestSSMStrategy_FetchVersion(t *testing.T) {
	t.Parallel()

	t.Run("success with version", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
				// Version selector uses GetParameter with name:version format
				return &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Name:    params.Name,
						Value:   lo.ToPtr("v2"),
						Version: 2,
					},
				}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		value, label, err := s.FetchVersion(context.Background(), "/app/param#2")
		require.NoError(t, err)
		assert.Equal(t, "v2", value)
		assert.Equal(t, "#2", label)
	})

	t.Run("success with shift", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
				return &ssm.GetParameterHistoryOutput{
					Parameters: []types.ParameterHistory{
						{Version: 1, Value: lo.ToPtr("v1")},
						{Version: 2, Value: lo.ToPtr("v2")},
						{Version: 3, Value: lo.ToPtr("v3")},
					},
				}, nil
			},
		}

		s := staging.NewSSMStrategy(mock)
		value, label, err := s.FetchVersion(context.Background(), "/app/param~1")
		require.NoError(t, err)
		assert.Equal(t, "v2", value)
		assert.Equal(t, "#2", label)
	})

	t.Run("parse error", func(t *testing.T) {
		t.Parallel()
		s := staging.NewSSMStrategy(&ssmMockClient{})
		_, _, err := s.FetchVersion(context.Background(), "invalid")
		require.Error(t, err)
	})

	t.Run("fetch error", func(t *testing.T) {
		t.Parallel()
		mock := &ssmMockClient{
			getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewSSMStrategy(mock)
		_, _, err := s.FetchVersion(context.Background(), "/app/param#2")
		require.Error(t, err)
	})
}

func TestSSMParserFactory(t *testing.T) {
	t.Parallel()

	parser := staging.SSMParserFactory()
	require.NotNil(t, parser)
	assert.Equal(t, staging.ServiceSSM, parser.Service())
}
