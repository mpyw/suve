package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/staging"
)

type paramMockClient struct {
	getParameterFunc           func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
	getParameterHistoryFunc    func(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error)
	putParameterFunc           func(ctx context.Context, params *paramapi.PutParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error)
	deleteParameterFunc        func(ctx context.Context, params *paramapi.DeleteParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error)
	addTagsToResourceFunc      func(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error)
	removeTagsFromResourceFunc func(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error)
}

func (m *paramMockClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, errors.New("GetParameter not mocked")
}

func (m *paramMockClient) GetParameterHistory(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, errors.New("GetParameterHistory not mocked")
}

func (m *paramMockClient) PutParameter(ctx context.Context, params *paramapi.PutParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return nil, errors.New("PutParameter not mocked")
}

func (m *paramMockClient) DeleteParameter(ctx context.Context, params *paramapi.DeleteParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return nil, errors.New("DeleteParameter not mocked")
}

func (m *paramMockClient) AddTagsToResource(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
	if m.addTagsToResourceFunc != nil {
		return m.addTagsToResourceFunc(ctx, params, optFns...)
	}
	return &paramapi.AddTagsToResourceOutput{}, nil
}

func (m *paramMockClient) RemoveTagsFromResource(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsFromResourceFunc != nil {
		return m.removeTagsFromResourceFunc(ctx, params, optFns...)
	}
	return &paramapi.RemoveTagsFromResourceOutput{}, nil
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
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, &paramapi.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
				assert.Equal(t, "new-value", lo.FromPtr(params.Value))
				assert.Equal(t, paramapi.ParameterTypeString, params.Type)
				return &paramapi.PutParameterOutput{Version: 1}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
		})
		require.NoError(t, err)
	})

	t.Run("update operation - preserves type", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{
						Name:  lo.ToPtr("/app/param"),
						Value: lo.ToPtr("old-value"),
						Type:  paramapi.ParameterTypeSecureString,
					},
				}, nil
			},
			putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				assert.Equal(t, paramapi.ParameterTypeSecureString, params.Type)
				return &paramapi.PutParameterOutput{Version: 2}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.NoError(t, err)
	})

	t.Run("delete operation", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			deleteParameterFunc: func(_ context.Context, params *paramapi.DeleteParameterInput, _ ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
				assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
				return &paramapi.DeleteParameterOutput{}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)
	})

	t.Run("unknown operation", func(t *testing.T) {
		t.Parallel()
		s := staging.NewParamStrategy(&paramMockClient{})
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.Operation("unknown"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation")
	})

	t.Run("get parameter error (not ParameterNotFound)", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, errors.New("access denied")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get existing parameter")
	})

	t.Run("put parameter error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, &paramapi.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				return nil, errors.New("put failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set parameter")
	})

	t.Run("delete parameter error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			deleteParameterFunc: func(_ context.Context, _ *paramapi.DeleteParameterInput, _ ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
				return nil, errors.New("delete failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
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
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{
						Name:    lo.ToPtr("/app/param"),
						Value:   lo.ToPtr("current-value"),
						Version: 5,
					},
				}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchCurrent(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, "current-value", result.Value)
		assert.Equal(t, "#5", result.Identifier)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, errors.New("not found")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchCurrent(context.Background(), "/app/param")
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

func TestParamStrategy_FetchCurrentValue(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{
						Value:            lo.ToPtr("fetched-value"),
						LastModifiedDate: &now,
					},
				}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchCurrentValue(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, "fetched-value", result.Value)
		assert.Equal(t, now, result.LastModified)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchCurrentValue(context.Background(), "/app/param")
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
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, params *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				// Version selector uses GetParameter with name:version format
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{
						Name:    params.Name,
						Value:   lo.ToPtr("v2"),
						Version: 2,
					},
				}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		value, label, err := s.FetchVersion(context.Background(), "/app/param#2")
		require.NoError(t, err)
		assert.Equal(t, "v2", value)
		assert.Equal(t, "#2", label)
	})

	t.Run("success with shift", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
				return &paramapi.GetParameterHistoryOutput{
					Parameters: []paramapi.ParameterHistory{
						{Version: 1, Value: lo.ToPtr("v1")},
						{Version: 2, Value: lo.ToPtr("v2")},
						{Version: 3, Value: lo.ToPtr("v3")},
					},
				}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		value, label, err := s.FetchVersion(context.Background(), "/app/param~1")
		require.NoError(t, err)
		assert.Equal(t, "v2", value)
		assert.Equal(t, "#2", label)
	})

	t.Run("parse error", func(t *testing.T) {
		t.Parallel()
		s := staging.NewParamStrategy(&paramMockClient{})
		_, _, err := s.FetchVersion(context.Background(), "invalid")
		require.Error(t, err)
	})

	t.Run("fetch error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
				return nil, errors.New("fetch error")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, _, err := s.FetchVersion(context.Background(), "/app/param#2")
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
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{
						Name:             lo.ToPtr("/app/param"),
						LastModifiedDate: &now,
					},
				}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.Equal(t, now, result)
	})

	t.Run("not found - returns zero time", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, &paramapi.ParameterNotFound{}
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})

	t.Run("other error - returns error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, errors.New("access denied")
			},
		}

		s := staging.NewParamStrategy(mock)
		_, err := s.FetchLastModified(context.Background(), "/app/param")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get parameter")
	})

	t.Run("nil LastModifiedDate - returns zero time", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{
						Name:             lo.ToPtr("/app/param"),
						LastModifiedDate: nil,
					},
				}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})

	t.Run("nil Parameter - returns zero time", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{Parameter: nil}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		result, err := s.FetchLastModified(context.Background(), "/app/param")
		require.NoError(t, err)
		assert.True(t, result.IsZero())
	})
}

func TestParamStrategy_Apply_WithTags(t *testing.T) {
	t.Parallel()

	t.Run("create with description", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, &paramapi.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				assert.Equal(t, "Test description", lo.FromPtr(params.Description))
				return &paramapi.PutParameterOutput{Version: 1}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation:   staging.OperationCreate,
			Value:       lo.ToPtr("value"),
			Description: lo.ToPtr("Test description"),
		})
		require.NoError(t, err)
	})

	t.Run("create with tags", func(t *testing.T) {
		t.Parallel()
		addTagsCalled := false
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, &paramapi.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				return &paramapi.PutParameterOutput{Version: 1}, nil
			},
			addTagsToResourceFunc: func(_ context.Context, params *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
				addTagsCalled = true
				assert.Len(t, params.Tags, 1)
				return &paramapi.AddTagsToResourceOutput{}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value"),
			Tags:      map[string]string{"env": "test"},
		})
		require.NoError(t, err)
		assert.True(t, addTagsCalled)
	})

	t.Run("update with untag keys", func(t *testing.T) {
		t.Parallel()
		removeTagsCalled := false
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return &paramapi.GetParameterOutput{
					Parameter: &paramapi.Parameter{Type: paramapi.ParameterTypeString},
				}, nil
			},
			putParameterFunc: func(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				return &paramapi.PutParameterOutput{Version: 2}, nil
			},
			removeTagsFromResourceFunc: func(_ context.Context, params *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
				removeTagsCalled = true
				assert.Contains(t, params.TagKeys, "old-tag")
				return &paramapi.RemoveTagsFromResourceOutput{}, nil
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated"),
			UntagKeys: []string{"old-tag"},
		})
		require.NoError(t, err)
		assert.True(t, removeTagsCalled)
	})

	t.Run("tagging error", func(t *testing.T) {
		t.Parallel()
		mock := &paramMockClient{
			getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
				return nil, &paramapi.ParameterNotFound{}
			},
			putParameterFunc: func(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
				return &paramapi.PutParameterOutput{Version: 1}, nil
			},
			addTagsToResourceFunc: func(_ context.Context, _ *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
				return nil, errors.New("tagging failed")
			},
		}

		s := staging.NewParamStrategy(mock)
		err := s.Apply(context.Background(), "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value"),
			Tags:      map[string]string{"env": "test"},
		})
		require.Error(t, err)
	})
}

func TestParamStrategy_Apply_DeleteAlreadyDeleted(t *testing.T) {
	t.Parallel()

	mock := &paramMockClient{
		deleteParameterFunc: func(_ context.Context, _ *paramapi.DeleteParameterInput, _ ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
			return nil, &paramapi.ParameterNotFound{}
		},
	}

	s := staging.NewParamStrategy(mock)
	err := s.Apply(context.Background(), "/app/param", staging.Entry{
		Operation: staging.OperationDelete,
	})
	require.NoError(t, err) // Should succeed even if already deleted
}

func TestParamStrategy_FetchCurrentValue_NoLastModified(t *testing.T) {
	t.Parallel()

	mock := &paramMockClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Value:            lo.ToPtr("value"),
					LastModifiedDate: nil,
				},
			}, nil
		},
	}

	s := staging.NewParamStrategy(mock)
	result, err := s.FetchCurrentValue(context.Background(), "/app/param")
	require.NoError(t, err)
	assert.Equal(t, "value", result.Value)
	assert.True(t, result.LastModified.IsZero())
}
