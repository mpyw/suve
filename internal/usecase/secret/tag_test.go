package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockTagClient struct {
	describeResult *secretapi.DescribeSecretOutput
	describeErr    error
	tagErr         error
	untagErr       error
}

func (m *mockTagClient) DescribeSecret(_ context.Context, _ *secretapi.DescribeSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	return m.describeResult, nil
}

func (m *mockTagClient) TagResource(_ context.Context, _ *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
	if m.tagErr != nil {
		return nil, m.tagErr
	}
	return &secretapi.TagResourceOutput{}, nil
}

func (m *mockTagClient) UntagResource(_ context.Context, _ *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
	if m.untagErr != nil {
		return nil, m.untagErr
	}
	return &secretapi.UntagResourceOutput{}, nil
}

func TestTagUseCase_Execute_AddTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeResult: &secretapi.DescribeSecretOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
		Add:  map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeResult: &secretapi.DescribeSecretOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Remove: []string{"old-tag", "deprecated"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_AddAndRemoveTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeResult: &secretapi.DescribeSecretOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Add:    map[string]string{"env": "prod"},
		Remove: []string{"old-tag"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_NoTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeResult: &secretapi.DescribeSecretOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_DescribeError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeErr: errors.New("describe failed"),
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
		Add:  map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe secret")
}

func TestTagUseCase_Execute_AddTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeResult: &secretapi.DescribeSecretOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
		tagErr: errors.New("tag failed"),
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
		Add:  map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestTagUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		describeResult: &secretapi.DescribeSecretOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
		untagErr: errors.New("untag failed"),
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Remove: []string{"old-tag"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}
