package tagging

import (
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/maputil"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		untags       []string
		wantAdd      map[string]string
		wantRemove   maputil.Set[string]
		wantWarnings []string
		wantErr      string
	}{
		{
			name:       "empty",
			tags:       nil,
			untags:     nil,
			wantAdd:    map[string]string{},
			wantRemove: maputil.NewSet[string](),
		},
		{
			name:       "add single tag",
			tags:       []string{"env=prod"},
			wantAdd:    map[string]string{"env": "prod"},
			wantRemove: maputil.NewSet[string](),
		},
		{
			name:       "add multiple tags",
			tags:       []string{"env=prod", "team=platform"},
			wantAdd:    map[string]string{"env": "prod", "team": "platform"},
			wantRemove: maputil.NewSet[string](),
		},
		{
			name:       "remove single tag",
			untags:     []string{"env"},
			wantAdd:    map[string]string{},
			wantRemove: maputil.NewSet("env"),
		},
		{
			name:       "remove multiple tags",
			untags:     []string{"env", "team"},
			wantAdd:    map[string]string{},
			wantRemove: maputil.NewSet("env", "team"),
		},
		{
			name:       "add and remove different tags",
			tags:       []string{"env=prod"},
			untags:     []string{"deprecated"},
			wantAdd:    map[string]string{"env": "prod"},
			wantRemove: maputil.NewSet("deprecated"),
		},
		{
			name:         "conflict - untag wins over tag",
			tags:         []string{"env=prod"},
			untags:       []string{"env"},
			wantAdd:      map[string]string{},
			wantRemove:   maputil.NewSet("env"),
			wantWarnings: []string{`tag "env": --untag env overrides --tag env=prod`},
		},
		{
			name:       "tag with equals in value",
			tags:       []string{"config=key=value"},
			wantAdd:    map[string]string{"config": "key=value"},
			wantRemove: maputil.NewSet[string](),
		},
		{
			name:       "tag with empty value",
			tags:       []string{"empty="},
			wantAdd:    map[string]string{"empty": ""},
			wantRemove: maputil.NewSet[string](),
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
			wantRemove:   maputil.NewSet[string](),
			wantWarnings: []string{`tag "env": --tag env=prod overrides --tag env=dev`},
		},
		{
			name:         "duplicate untag - last wins with warning",
			untags:       []string{"env", "env"},
			wantAdd:      map[string]string{},
			wantRemove:   maputil.NewSet("env"),
			wantWarnings: []string{`tag "env": --untag env overrides --untag env`},
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
			assert.ElementsMatch(t, tt.wantRemove.Values(), result.Change.Remove.Values())

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
			change: &Change{Add: map[string]string{}, Remove: maputil.NewSet[string]()},
			want:   true,
		},
		{
			name:   "has add",
			change: &Change{Add: map[string]string{"k": "v"}, Remove: maputil.NewSet[string]()},
			want:   false,
		},
		{
			name:   "has remove",
			change: &Change{Add: map[string]string{}, Remove: maputil.NewSet("k")},
			want:   false,
		},
		{
			name:   "has both",
			change: &Change{Add: map[string]string{"k": "v"}, Remove: maputil.NewSet("x")},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.change.IsEmpty())
		})
	}
}

// Mock Secrets Manager client for testing.
type mockSecretClient struct {
	tagResourceFunc   func(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error)
	untagResourceFunc func(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error)
}

func (m *mockSecretClient) TagResource(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
	if m.tagResourceFunc != nil {
		return m.tagResourceFunc(ctx, params, optFns...)
	}

	return &secretapi.TagResourceOutput{}, nil
}

func (m *mockSecretClient) UntagResource(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
	if m.untagResourceFunc != nil {
		return m.untagResourceFunc(ctx, params, optFns...)
	}

	return &secretapi.UntagResourceOutput{}, nil
}

func TestApplySecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		secretID string
		change   *Change
		mock     *mockSecretClient
		wantErr  string
	}{
		{
			name:     "empty change does nothing",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{}, Remove: maputil.NewSet[string]()},
			mock:     &mockSecretClient{},
		},
		{
			name:     "add tags",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{"env": "prod", "team": "platform"}, Remove: maputil.NewSet[string]()},
			mock: &mockSecretClient{
				tagResourceFunc: func(_ context.Context, params *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Len(t, params.Tags, 2)

					tagMap := make(map[string]string)
					for _, tag := range params.Tags {
						tagMap[lo.FromPtr(tag.Key)] = lo.FromPtr(tag.Value)
					}

					assert.Equal(t, "prod", tagMap["env"])
					assert.Equal(t, "platform", tagMap["team"])

					return &secretapi.TagResourceOutput{}, nil
				},
			},
		},
		{
			name:     "remove tags",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{}, Remove: maputil.NewSet("deprecated", "old")},
			mock: &mockSecretClient{
				untagResourceFunc: func(_ context.Context, params *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.ElementsMatch(t, []string{"deprecated", "old"}, params.TagKeys)

					return &secretapi.UntagResourceOutput{}, nil
				},
			},
		},
		{
			name:     "add and remove tags",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{"env": "prod"}, Remove: maputil.NewSet("deprecated")},
			mock: &mockSecretClient{
				tagResourceFunc: func(_ context.Context, params *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Len(t, params.Tags, 1)
					assert.Equal(t, "env", lo.FromPtr(params.Tags[0].Key))
					assert.Equal(t, "prod", lo.FromPtr(params.Tags[0].Value))

					return &secretapi.TagResourceOutput{}, nil
				},
				untagResourceFunc: func(_ context.Context, params *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Equal(t, []string{"deprecated"}, params.TagKeys)

					return &secretapi.UntagResourceOutput{}, nil
				},
			},
		},
		{
			name:     "tag resource error",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{"env": "prod"}, Remove: maputil.NewSet[string]()},
			mock: &mockSecretClient{
				tagResourceFunc: func(_ context.Context, _ *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to add tags",
		},
		{
			name:     "untag resource error",
			secretID: "my-secret",
			change:   &Change{Add: map[string]string{}, Remove: maputil.NewSet("deprecated")},
			mock: &mockSecretClient{
				untagResourceFunc: func(_ context.Context, _ *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ApplySecret(t.Context(), tt.mock, tt.secretID, tt.change)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

// Mock SSM Parameter Store client for testing.
type mockParamClient struct {
	addTagsToResourceFunc      func(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error)
	removeTagsFromResourceFunc func(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error)
}

func (m *mockParamClient) AddTagsToResource(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
	if m.addTagsToResourceFunc != nil {
		return m.addTagsToResourceFunc(ctx, params, optFns...)
	}

	return &paramapi.AddTagsToResourceOutput{}, nil
}

func (m *mockParamClient) RemoveTagsFromResource(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsFromResourceFunc != nil {
		return m.removeTagsFromResourceFunc(ctx, params, optFns...)
	}

	return &paramapi.RemoveTagsFromResourceOutput{}, nil
}

func TestApplyParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		resourceID string
		change     *Change
		mock       *mockParamClient
		wantErr    string
	}{
		{
			name:       "empty change does nothing",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{}, Remove: maputil.NewSet[string]()},
			mock:       &mockParamClient{},
		},
		{
			name:       "add tags",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{"env": "prod", "team": "platform"}, Remove: maputil.NewSet[string]()},
			mock: &mockParamClient{
				addTagsToResourceFunc: func(_ context.Context, params *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
					assert.Equal(t, paramapi.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.Len(t, params.Tags, 2)

					tagMap := make(map[string]string)
					for _, tag := range params.Tags {
						tagMap[lo.FromPtr(tag.Key)] = lo.FromPtr(tag.Value)
					}

					assert.Equal(t, "prod", tagMap["env"])
					assert.Equal(t, "platform", tagMap["team"])

					return &paramapi.AddTagsToResourceOutput{}, nil
				},
			},
		},
		{
			name:       "remove tags",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{}, Remove: maputil.NewSet("deprecated", "old")},
			mock: &mockParamClient{
				removeTagsFromResourceFunc: func(_ context.Context, params *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
					assert.Equal(t, paramapi.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.ElementsMatch(t, []string{"deprecated", "old"}, params.TagKeys)

					return &paramapi.RemoveTagsFromResourceOutput{}, nil
				},
			},
		},
		{
			name:       "add and remove tags",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{"env": "prod"}, Remove: maputil.NewSet("deprecated")},
			mock: &mockParamClient{
				addTagsToResourceFunc: func(_ context.Context, params *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
					assert.Equal(t, paramapi.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.Len(t, params.Tags, 1)
					assert.Equal(t, "env", lo.FromPtr(params.Tags[0].Key))
					assert.Equal(t, "prod", lo.FromPtr(params.Tags[0].Value))

					return &paramapi.AddTagsToResourceOutput{}, nil
				},
				removeTagsFromResourceFunc: func(_ context.Context, params *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
					assert.Equal(t, paramapi.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, "/my/param", lo.FromPtr(params.ResourceId))
					assert.Equal(t, []string{"deprecated"}, params.TagKeys)

					return &paramapi.RemoveTagsFromResourceOutput{}, nil
				},
			},
		},
		{
			name:       "add tags error",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{"env": "prod"}, Remove: maputil.NewSet[string]()},
			mock: &mockParamClient{
				addTagsToResourceFunc: func(_ context.Context, _ *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to add tags",
		},
		{
			name:       "remove tags error",
			resourceID: "/my/param",
			change:     &Change{Add: map[string]string{}, Remove: maputil.NewSet("deprecated")},
			mock: &mockParamClient{
				removeTagsFromResourceFunc: func(_ context.Context, _ *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ApplyParam(t.Context(), tt.mock, tt.resourceID, tt.change)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

// Verify secretapi.Tag is used correctly (compile-time check).
var _ secretapi.Tag = secretapi.Tag{}

// Verify paramapi.Tag is used correctly (compile-time check).
var _ paramapi.Tag = paramapi.Tag{}
