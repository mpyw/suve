package resource_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/resource"
)

type mockTagger struct {
	getTags    func(ctx context.Context, name string) (map[string]string, error)
	addTags    func(ctx context.Context, name string, tags map[string]string) error
	removeTags func(ctx context.Context, name string, keys []string) error
}

func (m *mockTagger) GetTags(ctx context.Context, name string) (map[string]string, error) {
	if m.getTags != nil {
		return m.getTags(ctx, name)
	}

	return map[string]string{}, nil
}

func (m *mockTagger) AddTags(ctx context.Context, name string, tags map[string]string) error {
	if m.addTags != nil {
		return m.addTags(ctx, name, tags)
	}

	return nil
}

func (m *mockTagger) RemoveTags(ctx context.Context, name string, keys []string) error {
	if m.removeTags != nil {
		return m.removeTags(ctx, name, keys)
	}

	return nil
}

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		resource *model.Resource
		tags     map[string]string
		tagErr   error
		wantTags []resource.ShowTag
	}{
		{
			name: "parameter resource with tags",
			resource: &model.Resource{
				Kind:        model.KindParameter,
				Name:        "/app/config",
				Value:       "secret-value",
				Version:     "5",
				Type:        "SecureString",
				Description: "Test parameter",
				ModifiedAt:  &modified,
				Metadata:    model.AWSParameterMeta{ARN: "arn:aws:ssm:us-east-1:123456789012:parameter/app/config"},
			},
			tags: map[string]string{"env": "prod", "team": "platform"},
			wantTags: []resource.ShowTag{
				{Key: "env", Value: "prod"},
				{Key: "team", Value: "platform"},
			},
		},
		{
			name: "secret resource with tags",
			resource: &model.Resource{
				Kind:        model.KindSecret,
				Name:        "my-secret",
				ARN:         "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
				Value:       "secret-value",
				Version:     "abc123",
				Description: "Test secret",
				ModifiedAt:  &modified,
				Metadata:    model.AWSSecretMeta{VersionStages: []string{"AWSCURRENT"}},
			},
			tags: map[string]string{"env": "staging"},
			wantTags: []resource.ShowTag{
				{Key: "env", Value: "staging"},
			},
		},
		{
			name: "resource without tags",
			resource: &model.Resource{
				Kind:        model.KindParameter,
				Name:        "/app/config",
				Value:       "value",
				Version:     "1",
				Description: "",
				ModifiedAt:  &modified,
			},
			tags:     nil,
			wantTags: nil,
		},
		{
			name: "tag fetch error is ignored",
			resource: &model.Resource{
				Kind:        model.KindParameter,
				Name:        "/app/config",
				Value:       "value",
				Version:     "1",
				Description: "",
				ModifiedAt:  &modified,
			},
			tagErr:   errors.New("access denied"),
			wantTags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &mockTagger{
				getTags: func(_ context.Context, _ string) (map[string]string, error) {
					return tt.tags, tt.tagErr
				},
			}

			uc := &resource.ShowUseCase{Client: client}
			output, err := uc.Execute(context.Background(), resource.ShowInput{
				Resource: tt.resource,
			})

			require.NoError(t, err)
			assert.Equal(t, tt.resource.Kind, output.Kind)
			assert.Equal(t, tt.resource.Name, output.Name)
			assert.Equal(t, tt.resource.ARN, output.ARN)
			assert.Equal(t, tt.resource.Value, output.Value)
			assert.Equal(t, tt.resource.Version, output.Version)
			assert.Equal(t, tt.resource.Type, output.Type)
			assert.Equal(t, tt.resource.Description, output.Description)
			assert.Equal(t, tt.resource.ModifiedAt, output.ModifiedAt)
			assert.Equal(t, tt.resource.Metadata, output.Metadata)

			if tt.wantTags == nil {
				assert.Nil(t, output.Tags)
			} else {
				assert.ElementsMatch(t, tt.wantTags, output.Tags)
			}
		})
	}
}
