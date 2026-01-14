package secret_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

type mockDiffClient struct {
	getSecretValueResults []*secretapi.GetSecretValueOutput
	getSecretValueErrs    []error
	getSecretValueCalls   int
	listVersionsResult    *secretapi.ListSecretVersionIDsOutput
	listVersionsErr       error
}

func (m *mockDiffClient) GetSecretValue(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	idx := m.getSecretValueCalls
	m.getSecretValueCalls++

	if idx < len(m.getSecretValueErrs) && m.getSecretValueErrs[idx] != nil {
		return nil, m.getSecretValueErrs[idx]
	}

	if idx < len(m.getSecretValueResults) {
		return m.getSecretValueResults[idx], nil
	}

	return nil, errors.New("unexpected GetSecretValue call")
}

//nolint:revive,stylecheck // Method name must match AWS SDK interface
func (m *mockDiffClient) ListSecretVersionIds(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
	if m.listVersionsErr != nil {
		return nil, m.listVersionsErr
	}

	return m.listVersionsResult, nil
}

func TestDiffUseCase_Execute(t *testing.T) {
	t.Parallel()

	// Specs without shift use GetSecretValue directly
	client := &mockDiffClient{
		getSecretValueResults: []*secretapi.GetSecretValueOutput{
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v1-id"), SecretString: lo.ToPtr("old-value")},
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v2-id"), SecretString: lo.ToPtr("new-value")},
		},
	}

	uc := &secret.DiffUseCase{Client: client}

	spec1, _ := secretversion.Parse("my-secret#v1-id")
	spec2, _ := secretversion.Parse("my-secret#v2-id")

	output, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.OldName)
	assert.Equal(t, "v1-id", output.OldVersionID)
	assert.Equal(t, "old-value", output.OldValue)
	assert.Equal(t, "my-secret", output.NewName)
	assert.Equal(t, "v2-id", output.NewVersionID)
	assert.Equal(t, "new-value", output.NewValue)
}

func TestDiffUseCase_Execute_Spec1Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getSecretValueErrs: []error{errors.New("get secret error")},
	}

	uc := &secret.DiffUseCase{Client: client}

	spec1, _ := secretversion.Parse("my-secret#v1-id")
	spec2, _ := secretversion.Parse("my-secret#v2-id")

	_, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_Spec2Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getSecretValueResults: []*secretapi.GetSecretValueOutput{
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v1-id"), SecretString: lo.ToPtr("old-value")},
		},
		getSecretValueErrs: []error{nil, errors.New("second get secret error")},
	}

	uc := &secret.DiffUseCase{Client: client}

	spec1, _ := secretversion.Parse("my-secret#v1-id")
	spec2, _ := secretversion.Parse("my-secret#v2-id")

	_, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_WithLabel(t *testing.T) {
	t.Parallel()

	// Specs with label use GetSecretValue directly
	client := &mockDiffClient{
		getSecretValueResults: []*secretapi.GetSecretValueOutput{
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v1-id"), SecretString: lo.ToPtr("previous-value")},
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v2-id"), SecretString: lo.ToPtr("current-value")},
		},
	}

	uc := &secret.DiffUseCase{Client: client}

	spec1, _ := secretversion.Parse("my-secret:AWSPREVIOUS")
	spec2, _ := secretversion.Parse("my-secret:AWSCURRENT")

	output, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1-id", output.OldVersionID)
	assert.Equal(t, "v2-id", output.NewVersionID)
}

func TestDiffUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Specs with shift use ListSecretVersionIds + GetSecretValue
	client := &mockDiffClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueResults: []*secretapi.GetSecretValueOutput{
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v1-id"), SecretString: lo.ToPtr("v1-value")},
			{Name: lo.ToPtr("my-secret"), VersionId: lo.ToPtr("v2-id"), SecretString: lo.ToPtr("v2-value")},
		},
	}

	uc := &secret.DiffUseCase{Client: client}

	spec1, _ := secretversion.Parse("my-secret~2") // 2 versions back from latest
	spec2, _ := secretversion.Parse("my-secret~1") // 1 version back from latest

	output, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, "v1-id", output.OldVersionID)
	assert.Equal(t, "v1-value", output.OldValue)
	assert.Equal(t, "v2-id", output.NewVersionID)
	assert.Equal(t, "v2-value", output.NewValue)
}
