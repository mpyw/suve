package tagging

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		untags       []string
		wantAdd      map[string]string
		wantRemove   []string
		wantWarnings []string
		wantErr      string
	}{
		{
			name:       "empty",
			tags:       nil,
			untags:     nil,
			wantAdd:    map[string]string{},
			wantRemove: []string{},
		},
		{
			name:       "add single tag",
			tags:       []string{"env=prod"},
			wantAdd:    map[string]string{"env": "prod"},
			wantRemove: []string{},
		},
		{
			name:       "add multiple tags",
			tags:       []string{"env=prod", "team=platform"},
			wantAdd:    map[string]string{"env": "prod", "team": "platform"},
			wantRemove: []string{},
		},
		{
			name:       "remove single tag",
			untags:     []string{"env"},
			wantAdd:    map[string]string{},
			wantRemove: []string{"env"},
		},
		{
			name:       "remove multiple tags",
			untags:     []string{"env", "team"},
			wantAdd:    map[string]string{},
			wantRemove: []string{"env", "team"},
		},
		{
			name:       "add and remove different tags",
			tags:       []string{"env=prod"},
			untags:     []string{"deprecated"},
			wantAdd:    map[string]string{"env": "prod"},
			wantRemove: []string{"deprecated"},
		},
		{
			name:         "conflict - untag wins over tag",
			tags:         []string{"env=prod"},
			untags:       []string{"env"},
			wantAdd:      map[string]string{},
			wantRemove:   []string{"env"},
			wantWarnings: []string{`tag "env": --untag env overrides --tag env=prod`},
		},
		{
			name:       "tag with equals in value",
			tags:       []string{"config=key=value"},
			wantAdd:    map[string]string{"config": "key=value"},
			wantRemove: []string{},
		},
		{
			name:       "tag with empty value",
			tags:       []string{"empty="},
			wantAdd:    map[string]string{"empty": ""},
			wantRemove: []string{},
		},
		{
			name:    "invalid tag format - no equals",
			tags:    []string{"invalid"},
			wantErr: `invalid tag format "invalid": expected key=value`,
		},
		{
			name:    "invalid tag format - empty key",
			tags:    []string{"=value"},
			wantErr: `invalid tag format "=value": key cannot be empty`,
		},
		{
			name:    "invalid untag - empty key",
			untags:  []string{""},
			wantErr: "invalid untag: key cannot be empty",
		},
		{
			name:         "duplicate tag - last wins",
			tags:         []string{"env=dev", "env=prod"},
			wantAdd:      map[string]string{"env": "prod"},
			wantRemove:   []string{},
			wantWarnings: []string{`tag "env": --tag env=prod overrides --tag env=dev`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFlags(tt.tags, tt.untags)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAdd, result.Change.Add)
			assert.ElementsMatch(t, tt.wantRemove, result.Change.Remove)
			if tt.wantWarnings == nil {
				assert.Empty(t, result.Warnings)
			} else {
				assert.Equal(t, tt.wantWarnings, result.Warnings)
			}
		})
	}
}

func TestChange_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		change *Change
		want   bool
	}{
		{
			name:   "empty",
			change: &Change{Add: map[string]string{}, Remove: []string{}},
			want:   true,
		},
		{
			name:   "has add",
			change: &Change{Add: map[string]string{"k": "v"}, Remove: []string{}},
			want:   false,
		},
		{
			name:   "has remove",
			change: &Change{Add: map[string]string{}, Remove: []string{"k"}},
			want:   false,
		},
		{
			name:   "has both",
			change: &Change{Add: map[string]string{"k": "v"}, Remove: []string{"x"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.change.IsEmpty())
		})
	}
}

// Mock SM client for testing
type mockSMClient struct {
	tagResourceFunc   func(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error)
	untagResourceFunc func(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error)
}

func (m *mockSMClient) TagResource(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
	if m.tagResourceFunc != nil {
		return m.tagResourceFunc(ctx, params, optFns...)
	}
	return &secretsmanager.TagResourceOutput{}, nil
}

func (m *mockSMClient) UntagResource(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
	if m.untagResourceFunc != nil {
		return m.untagResourceFunc(ctx, params, optFns...)
	}
	return &secretsmanager.UntagResourceOutput{}, nil
}

func TestApplySM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		secretID string
		change   *Change
		mock     *mockSMClient
		wantErr  string
	}{
		{
			name:     "empty change does nothing",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{}, Remove: []string{}},
			mock:     &mockSMClient{},
		},
		{
			name:     "add tags",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{"env": "prod", "team": "platform"}, Remove: []string{}},
			mock: &mockSMClient{
				tagResourceFunc: func(_ context.Context, params *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Len(t, params.Tags, 2)
					tagMap := make(map[string]string)
					for _, tag := range params.Tags {
						tagMap[lo.FromPtr(tag.Key)] = lo.FromPtr(tag.Value)
					}
					assert.Equal(t, "prod", tagMap["env"])
					assert.Equal(t, "platform", tagMap["team"])
					return &secretsmanager.TagResourceOutput{}, nil
				},
			},
		},
		{
			name:     "remove tags",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{}, Remove: []string{"deprecated", "old"}},
			mock: &mockSMClient{
				untagResourceFunc: func(_ context.Context, params *secretsmanager.UntagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.ElementsMatch(t, []string{"deprecated", "old"}, params.TagKeys)
					return &secretsmanager.UntagResourceOutput{}, nil
				},
			},
		},
		{
			name:     "add and remove tags",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{"env": "prod"}, Remove: []string{"deprecated"}},
			mock: &mockSMClient{
				tagResourceFunc: func(_ context.Context, params *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Len(t, params.Tags, 1)
					assert.Equal(t, "env", lo.FromPtr(params.Tags[0].Key))
					assert.Equal(t, "prod", lo.FromPtr(params.Tags[0].Value))
					return &secretsmanager.TagResourceOutput{}, nil
				},
				untagResourceFunc: func(_ context.Context, params *secretsmanager.UntagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Equal(t, []string{"deprecated"}, params.TagKeys)
					return &secretsmanager.UntagResourceOutput{}, nil
				},
			},
		},
		{
			name:     "tag resource error",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{"env": "prod"}, Remove: []string{}},
			mock: &mockSMClient{
				tagResourceFunc: func(_ context.Context, _ *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to add tags",
		},
		{
			name:     "untag resource error",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{}, Remove: []string{"deprecated"}},
			mock: &mockSMClient{
				untagResourceFunc: func(_ context.Context, _ *secretsmanager.UntagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ApplySM(t.Context(), tt.mock, tt.secretID, tt.change)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

// Mock SSM client for testing
type mockSSMClient struct {
	addTagsToResourceFunc      func(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error)
	removeTagsFromResourceFunc func(ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error)
}

func (m *mockSSMClient) AddTagsToResource(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error) {
	if m.addTagsToResourceFunc != nil {
		return m.addTagsToResourceFunc(ctx, params, optFns...)
	}
	return &ssm.AddTagsToResourceOutput{}, nil
}

func (m *mockSSMClient) RemoveTagsFromResource(ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsFromResourceFunc != nil {
		return m.removeTagsFromResourceFunc(ctx, params, optFns...)
	}
	return &ssm.RemoveTagsFromResourceOutput{}, nil
}

func TestApplySSM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		resourceID string
		change     *Change
		mock       *mockSSMClient
		wantErr    string
	}{
		{
			name:       "empty change does nothing",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{}, Remove: []string{}},
			mock:       &mockSSMClient{},
		},
		{
			name:       "add tags",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{"env": "prod", "team": "platform"}, Remove: []string{}},
			mock: &mockSSMClient{
				addTagsToResourceFunc: func(_ context.Context, params *ssm.AddTagsToResourceInput, _ ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error) {
					assert.Equal(t, ssmtypes.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.Len(t, params.Tags, 2)
					tagMap := make(map[string]string)
					for _, tag := range params.Tags {
						tagMap[lo.FromPtr(tag.Key)] = lo.FromPtr(tag.Value)
					}
					assert.Equal(t, "prod", tagMap["env"])
					assert.Equal(t, "platform", tagMap["team"])
					return &ssm.AddTagsToResourceOutput{}, nil
				},
			},
		},
		{
			name:       "remove tags",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{}, Remove: []string{"deprecated", "old"}},
			mock: &mockSSMClient{
				removeTagsFromResourceFunc: func(_ context.Context, params *ssm.RemoveTagsFromResourceInput, _ ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error) {
					assert.Equal(t, ssmtypes.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.ElementsMatch(t, []string{"deprecated", "old"}, params.TagKeys)
					return &ssm.RemoveTagsFromResourceOutput{}, nil
				},
			},
		},
		{
			name:       "add and remove tags",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{"env": "prod"}, Remove: []string{"deprecated"}},
			mock: &mockSSMClient{
				addTagsToResourceFunc: func(_ context.Context, params *ssm.AddTagsToResourceInput, _ ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error) {
					assert.Equal(t, ssmtypes.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.Len(t, params.Tags, 1)
					assert.Equal(t, "env", lo.FromPtr(params.Tags[0].Key))
					assert.Equal(t, "prod", lo.FromPtr(params.Tags[0].Value))
					return &ssm.AddTagsToResourceOutput{}, nil
				},
				removeTagsFromResourceFunc: func(_ context.Context, params *ssm.RemoveTagsFromResourceInput, _ ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error) {
					assert.Equal(t, ssmtypes.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.Equal(t, []string{"deprecated"}, params.TagKeys)
					return &ssm.RemoveTagsFromResourceOutput{}, nil
				},
			},
		},
		{
			name:       "add tags error",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{"env": "prod"}, Remove: []string{}},
			mock: &mockSSMClient{
				addTagsToResourceFunc: func(_ context.Context, _ *ssm.AddTagsToResourceInput, _ ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to add tags",
		},
		{
			name:       "remove tags error",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{}, Remove: []string{"deprecated"}},
			mock: &mockSSMClient{
				removeTagsFromResourceFunc: func(_ context.Context, _ *ssm.RemoveTagsFromResourceInput, _ ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ApplySSM(t.Context(), tt.mock, tt.resourceID, tt.change)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

// Verify smtypes.Tag is used correctly (compile-time check)
var _ smtypes.Tag = smtypes.Tag{}

// Verify ssmtypes.Tag is used correctly (compile-time check)
var _ ssmtypes.Tag = ssmtypes.Tag{}
