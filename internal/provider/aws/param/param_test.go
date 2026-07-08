package param_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/param"
)

// mockClient is a configurable mock of the narrow SSM client interface.
type mockClient struct {
	getParameter    func(*ssm.GetParameterInput) (*ssm.GetParameterOutput, error)
	getHistory      func(*ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error)
	putParameter    func(*ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
	deleteParameter func(*ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error)
	describe        func(*ssm.DescribeParametersInput) (*ssm.DescribeParametersOutput, error)
	addTags         func(*ssm.AddTagsToResourceInput) (*ssm.AddTagsToResourceOutput, error)
	removeTags      func(*ssm.RemoveTagsFromResourceInput) (*ssm.RemoveTagsFromResourceOutput, error)
	listTags        func(*ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error)
}

func (m *mockClient) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return m.getParameter(in)
}

func (m *mockClient) GetParameterHistory(
	_ context.Context, in *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options),
) (*ssm.GetParameterHistoryOutput, error) {
	return m.getHistory(in)
}

func (m *mockClient) PutParameter(_ context.Context, in *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	return m.putParameter(in)
}

func (m *mockClient) DeleteParameter(
	_ context.Context, in *ssm.DeleteParameterInput, _ ...func(*ssm.Options),
) (*ssm.DeleteParameterOutput, error) {
	return m.deleteParameter(in)
}

func (m *mockClient) DescribeParameters(
	_ context.Context, in *ssm.DescribeParametersInput, _ ...func(*ssm.Options),
) (*ssm.DescribeParametersOutput, error) {
	return m.describe(in)
}

func (m *mockClient) AddTagsToResource(
	_ context.Context, in *ssm.AddTagsToResourceInput, _ ...func(*ssm.Options),
) (*ssm.AddTagsToResourceOutput, error) {
	return m.addTags(in)
}

func (m *mockClient) RemoveTagsFromResource(
	_ context.Context, in *ssm.RemoveTagsFromResourceInput, _ ...func(*ssm.Options),
) (*ssm.RemoveTagsFromResourceOutput, error) {
	return m.removeTags(in)
}

func (m *mockClient) ListTagsForResource(
	_ context.Context, in *ssm.ListTagsForResourceInput, _ ...func(*ssm.Options),
) (*ssm.ListTagsForResourceOutput, error) {
	return m.listTags(in)
}

// historyOldestFirst returns 3 versions (oldest first, as AWS returns them).
func historyOldestFirst() []types.ParameterHistory {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	return []types.ParameterHistory{
		{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, Type: types.ParameterTypeString, LastModifiedDate: aws.Time(base)},
		{
			Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2,
			Type: types.ParameterTypeString, LastModifiedDate: aws.Time(base.Add(time.Hour)),
		},
		{
			Name: aws.String("/my/param"), Value: aws.String("v3"), Version: 3,
			Type: types.ParameterTypeString, LastModifiedDate: aws.Time(base.Add(2 * time.Hour)),
		},
	}
}

func TestResolve_Latest(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{})

	ref, err := store.Resolve(t.Context(), "/my/param", "")
	require.NoError(t, err)
	assert.True(t, ref.IsLatest())
	assert.Empty(t, ref.ID())
}

func TestResolve_AbsoluteVersion(t *testing.T) {
	t.Parallel()

	// No shift => no history call needed.
	store := param.New(&mockClient{})

	ref, err := store.Resolve(t.Context(), "/my/param", "#3")
	require.NoError(t, err)
	assert.False(t, ref.IsLatest())
	assert.Equal(t, "3", ref.ID())
}

func TestResolve_Shift(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{
		getHistory: func(_ *ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{Parameters: historyOldestFirst()}, nil
		},
	})

	// ~1 from latest (v3) => v2.
	ref, err := store.Resolve(t.Context(), "/my/param", "~1")
	require.NoError(t, err)
	assert.Equal(t, "2", ref.ID())
}

func TestResolve_VersionThenShift(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{
		getHistory: func(_ *ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{Parameters: historyOldestFirst()}, nil
		},
	})

	// #3~2 => v3 then 2 back => v1.
	ref, err := store.Resolve(t.Context(), "/my/param", "#3~2")
	require.NoError(t, err)
	assert.Equal(t, "1", ref.ID())
}

func TestResolve_ShiftOutOfRange(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{
		getHistory: func(_ *ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{Parameters: historyOldestFirst()}, nil
		},
	})

	_, err := store.Resolve(t.Context(), "/my/param", "~5")
	require.Error(t, err)
}

func TestGet_LatestWithTypeMappingAndTags(t *testing.T) {
	t.Parallel()

	var gotName string

	store := param.New(&mockClient{
		getParameter: func(in *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
			gotName = aws.ToString(in.Name)

			return &ssm.GetParameterOutput{Parameter: &types.Parameter{
				Name:             aws.String("/my/param"),
				Value:            aws.String("hunter2"),
				Version:          3,
				Type:             types.ParameterTypeSecureString,
				LastModifiedDate: aws.Time(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)),
			}}, nil
		},
		listTags: func(_ *ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error) {
			return &ssm.ListTagsForResourceOutput{TagList: []types.Tag{
				{Key: aws.String("env"), Value: aws.String("prod")},
			}}, nil
		},
	})

	entry, err := store.Get(t.Context(), "/my/param", provider.NewVersionRef(""))
	require.NoError(t, err)
	assert.Equal(t, "/my/param", gotName) // latest => no ":version" suffix
	assert.Equal(t, "hunter2", entry.Value)
	assert.Equal(t, domain.ValueTypeSecret, entry.Type)
	assert.Equal(t, "3", entry.Version.ID)
	require.Len(t, entry.Tags, 1)
	assert.Equal(t, "env", entry.Tags[0].Key)
	assert.Equal(t, "prod", entry.Tags[0].Value)
}

func TestGet_SpecificVersionSuffix(t *testing.T) {
	t.Parallel()

	var gotName string

	store := param.New(&mockClient{
		getParameter: func(in *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
			gotName = aws.ToString(in.Name)

			return &ssm.GetParameterOutput{Parameter: &types.Parameter{
				Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2, Type: types.ParameterTypeStringList,
			}}, nil
		},
		listTags: func(_ *ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error) {
			return &ssm.ListTagsForResourceOutput{}, nil
		},
	})

	entry, err := store.Get(t.Context(), "/my/param", provider.NewVersionRef("2"))
	require.NoError(t, err)
	assert.Equal(t, "/my/param:2", gotName)
	assert.Equal(t, domain.ValueTypeList, entry.Type)
}

func TestHistory_NewestFirst(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{
		getHistory: func(_ *ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{Parameters: historyOldestFirst()}, nil
		},
	})

	versions, err := store.History(t.Context(), "/my/param")
	require.NoError(t, err)
	require.Len(t, versions, 3)
	assert.Equal(t, "3", versions[0].ID)
	assert.Equal(t, "1", versions[2].ID)
}

func TestList_Paginated(t *testing.T) {
	t.Parallel()

	calls := 0
	store := param.New(&mockClient{
		describe: func(in *ssm.DescribeParametersInput) (*ssm.DescribeParametersOutput, error) {
			calls++

			if aws.ToString(in.NextToken) == "" {
				return &ssm.DescribeParametersOutput{
					Parameters: []types.ParameterMetadata{{Name: aws.String("/a")}},
					NextToken:  aws.String("tok"),
				}, nil
			}

			return &ssm.DescribeParametersOutput{
				Parameters: []types.ParameterMetadata{{Name: aws.String("/b")}},
			}, nil
		},
	})

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"/a", "/b"}, names)
	assert.Equal(t, 2, calls)
}

func TestPut_MapsTypeAndReturnsVersion(t *testing.T) {
	t.Parallel()

	var got *ssm.PutParameterInput

	store := param.New(&mockClient{
		putParameter: func(in *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
			got = in

			return &ssm.PutParameterOutput{Version: 7}, nil
		},
	})

	v, err := store.Put(t.Context(), "/my/param", "val", domain.ValueTypeSecret, "desc")
	require.NoError(t, err)
	assert.Equal(t, "7", v.ID)
	assert.Equal(t, types.ParameterTypeSecureString, got.Type)
	assert.True(t, aws.ToBool(got.Overwrite))
	assert.Equal(t, "desc", aws.ToString(got.Description))
}

func TestCreate_AppliesWriteOptions(t *testing.T) {
	t.Parallel()

	var got *ssm.PutParameterInput

	store := param.New(&mockClient{
		putParameter: func(in *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
			got = in

			return &ssm.PutParameterOutput{Version: 1}, nil
		},
	})

	_, err := store.Create(t.Context(), "/my/param", "v", domain.ValueTypePlaintext, "",
		param.Tier{Value: "Advanced"},
		param.DataType{Value: "aws:ec2:image"},
		param.AllowedPattern{Value: "^ami-"},
		param.Policies{JSON: `[{"Type":"Expiration"}]`},
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, types.ParameterTierAdvanced, got.Tier)
	assert.Equal(t, "aws:ec2:image", aws.ToString(got.DataType))
	assert.Equal(t, "^ami-", aws.ToString(got.AllowedPattern))
	assert.JSONEq(t, `[{"Type":"Expiration"}]`, aws.ToString(got.Policies))
}

func TestPut_AppliesWriteOptionsAndIgnoresUnknown(t *testing.T) {
	t.Parallel()

	var got *ssm.PutParameterInput

	store := param.New(&mockClient{
		putParameter: func(in *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
			got = in

			return &ssm.PutParameterOutput{Version: 2}, nil
		},
	})

	// unknownOption is not one this adapter understands; it must be ignored.
	_, err := store.Put(t.Context(), "/my/param", "v", domain.ValueTypePlaintext, "",
		param.Tier{Value: "Intelligent-Tiering"},
		unknownOption{},
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, types.ParameterTierIntelligentTiering, got.Tier)
	// Options that were not provided stay unset.
	assert.Nil(t, got.DataType)
	assert.Nil(t, got.AllowedPattern)
	assert.Nil(t, got.Policies)
}

// unknownOption is a WriteOption the param adapter does not recognize; it
// exercises the "ignore unknown options" branch of the pass-through contract.
type unknownOption struct{ provider.WriteOptionMarker }

func TestDelete(t *testing.T) {
	t.Parallel()

	var gotName string

	store := param.New(&mockClient{
		deleteParameter: func(in *ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error) {
			gotName = aws.ToString(in.Name)

			return &ssm.DeleteParameterOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "/my/param"))
	assert.Equal(t, "/my/param", gotName)
}

func TestTagAndUntag(t *testing.T) {
	t.Parallel()

	var (
		addIn    *ssm.AddTagsToResourceInput
		removeIn *ssm.RemoveTagsFromResourceInput
	)

	store := param.New(&mockClient{
		addTags: func(in *ssm.AddTagsToResourceInput) (*ssm.AddTagsToResourceOutput, error) {
			addIn = in

			return &ssm.AddTagsToResourceOutput{}, nil
		},
		removeTags: func(in *ssm.RemoveTagsFromResourceInput) (*ssm.RemoveTagsFromResourceOutput, error) {
			removeIn = in

			return &ssm.RemoveTagsFromResourceOutput{}, nil
		},
	})

	require.NoError(t, store.Tag(t.Context(), "/my/param", map[string]string{"env": "prod"}))
	require.NotNil(t, addIn)
	assert.Equal(t, types.ResourceTypeForTaggingParameter, addIn.ResourceType)
	require.Len(t, addIn.Tags, 1)

	require.NoError(t, store.Untag(t.Context(), "/my/param", []string{"env"}))
	require.NotNil(t, removeIn)
	assert.Equal(t, []string{"env"}, removeIn.TagKeys)
}

func TestCreate_NewReturnsVersion(t *testing.T) {
	t.Parallel()

	var putIn *ssm.PutParameterInput

	store := param.New(&mockClient{
		putParameter: func(in *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
			putIn = in

			return &ssm.PutParameterOutput{Version: 1}, nil
		},
	})

	got, err := store.Create(t.Context(), "/my/param", "v", domain.ValueTypePlaintext, "")
	require.NoError(t, err)
	assert.Equal(t, "1", got.ID)
	require.NotNil(t, putIn)
	assert.False(t, aws.ToBool(putIn.Overwrite))
}

func TestCreate_AlreadyExistsMapsSentinel(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{
		putParameter: func(*ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
			return nil, &types.ParameterAlreadyExists{Message: aws.String("exists")}
		},
	})

	_, err := store.Create(t.Context(), "/my/param", "v", domain.ValueTypePlaintext, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrAlreadyExists)
}

func TestGet_NotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := param.New(&mockClient{
		getParameter: func(*ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
			return nil, &types.ParameterNotFound{Message: aws.String("nope")}
		},
	})

	_, err := store.Get(t.Context(), "/my/param", provider.VersionRef{})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

// paginatedHistoryClient returns history across two pages: page 1 (versions 1,2)
// with a NextToken, page 2 (version 3) without. Exercises the NextToken loop.
func paginatedHistoryClient() *mockClient {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	return &mockClient{
		getHistory: func(in *ssm.GetParameterHistoryInput) (*ssm.GetParameterHistoryOutput, error) {
			if aws.ToString(in.NextToken) == "" {
				return &ssm.GetParameterHistoryOutput{
					Parameters: []types.ParameterHistory{
						{Name: aws.String("/my/param"), Version: 1, LastModifiedDate: aws.Time(base)},
						{Name: aws.String("/my/param"), Version: 2, LastModifiedDate: aws.Time(base.Add(time.Hour))},
					},
					NextToken: aws.String("tok"),
				}, nil
			}

			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: aws.String("/my/param"), Version: 3, LastModifiedDate: aws.Time(base.Add(2 * time.Hour))},
				},
			}, nil
		},
	}
}

// TestHistory_Paginated guards #311: History must page through NextToken so the
// newest version (on a later page) is not lost.
func TestHistory_Paginated(t *testing.T) {
	t.Parallel()

	versions, err := param.New(paginatedHistoryClient()).History(t.Context(), "/my/param")
	require.NoError(t, err)
	require.Len(t, versions, 3)
	assert.Equal(t, "3", versions[0].ID) // newest first, across pages
	assert.Equal(t, "1", versions[2].ID)
}

// TestResolve_ShiftAcrossPages guards #311: ~N must anchor at the true latest
// (last page), not at page 1's last entry.
func TestResolve_ShiftAcrossPages(t *testing.T) {
	t.Parallel()

	// ~1 from the true latest (v3, on page 2) => v2.
	ref, err := param.New(paginatedHistoryClient()).Resolve(t.Context(), "/my/param", "~1")
	require.NoError(t, err)
	assert.Equal(t, "2", ref.ID())

	// #3 exists only on page 2; #3~2 => v1.
	ref, err = param.New(paginatedHistoryClient()).Resolve(t.Context(), "/my/param", "#3~2")
	require.NoError(t, err)
	assert.Equal(t, "1", ref.ID())
}
