package secret_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

type mockDiffClient struct {
	getSecretResults []*model.Secret
	getSecretErrs    []error
	getSecretCalls   int
	versionsResult   []*model.SecretVersion
	versionsErr      error
}

func (m *mockDiffClient) GetSecret(_ context.Context, _, _, _ string) (*model.Secret, error) {
	idx := m.getSecretCalls
	m.getSecretCalls++

	if idx < len(m.getSecretErrs) && m.getSecretErrs[idx] != nil {
		return nil, m.getSecretErrs[idx]
	}

	if idx < len(m.getSecretResults) {
		return m.getSecretResults[idx], nil
	}

	return nil, errors.New("unexpected GetSecret call")
}

func (m *mockDiffClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	if m.versionsErr != nil {
		return nil, m.versionsErr
	}

	return m.versionsResult, nil
}

func (m *mockDiffClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	return nil, errors.New("not implemented")
}

func TestDiffUseCase_Execute(t *testing.T) {
	t.Parallel()

	// Specs without shift use GetSecret directly
	client := &mockDiffClient{
		getSecretResults: []*model.Secret{
			{Name: "my-secret", Version: "v1-id", Value: "old-value"},
			{Name: "my-secret", Version: "v2-id", Value: "new-value"},
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
		getSecretErrs: []error{errors.New("get secret error")},
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
		getSecretResults: []*model.Secret{
			{Name: "my-secret", Version: "v1-id", Value: "old-value"},
		},
		getSecretErrs: []error{nil, errors.New("second get secret error")},
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

	// Specs with label use GetSecret directly
	client := &mockDiffClient{
		getSecretResults: []*model.Secret{
			{Name: "my-secret", Version: "v1-id", Value: "previous-value"},
			{Name: "my-secret", Version: "v2-id", Value: "current-value"},
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
	// Specs with shift use GetSecretVersions + GetSecret
	client := &mockDiffClient{
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-2 * time.Hour))},
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
			{Version: "v3-id", CreatedAt: ptrTime(now)},
		},
		getSecretResults: []*model.Secret{
			{Name: "my-secret", Version: "v1-id", Value: "v1-value"},
			{Name: "my-secret", Version: "v2-id", Value: "v2-value"},
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

func ptrTime(t time.Time) *time.Time {
	return &t
}
