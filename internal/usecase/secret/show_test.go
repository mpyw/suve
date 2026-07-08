package secret_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// showStore builds a mock reader that resolves to the latest ref and returns the
// given entry.
func showStore(entry *domain.Entry) *providermock.Store {
	return &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return entry, nil
		},
	}
}

func mustParseSpec(t *testing.T, s string) *secretversion.Spec {
	t.Helper()

	spec, err := secretversion.Parse(s)
	require.NoError(t, err)

	return spec
}

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := showStore(&domain.Entry{
		Name:  "my-secret",
		Value: "secret-value",
		Type:  domain.ValueTypeSecret,
		// Multiple staging labels (unsorted) must all surface, deterministically
		// sorted for stable output (#419).
		Version: domain.Version{ID: "abc123", StagingLabels: []string{"AWSCURRENT", "custom"}, Created: &now},
		Extra:   []domain.Field{{Label: "ARN", Value: "arn:aws:secretsmanager:us-east-1:123:secret:my-secret"}},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret")})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "secret-value", output.Value)
	assert.Equal(t, "abc123", output.VersionID)
	assert.Equal(t, []string{"AWSCURRENT", "custom"}, output.VersionStage)
	assert.NotNil(t, output.CreatedDate)
	// Regression guard: the ARN must be surfaced from the entry's Extra metadata.
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:my-secret", output.ARN)
}

func TestShowUseCase_Execute_WithVersionID(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "old-value",
		Version: domain.Version{ID: "old-version-id", StagingLabels: []string{"AWSPREVIOUS"}},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret#old-version-id")})
	require.NoError(t, err)
	assert.Equal(t, "old-value", output.Value)
	assert.Equal(t, "old-version-id", output.VersionID)
}

func TestShowUseCase_Execute_WithLabel(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "current-value",
		Version: domain.Version{ID: "current-id", StagingLabels: []string{"AWSCURRENT"}},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret:AWSCURRENT")})
	require.NoError(t, err)
	assert.Equal(t, "current-value", output.Value)
}

func TestShowUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, assert.AnError
		},
	}

	uc := &secret.ShowUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret")})
	assert.Error(t, err)
}

func TestShowUseCase_Execute_NoCreatedDate(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "secret-value",
		Version: domain.Version{ID: "abc123"},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret")})
	require.NoError(t, err)
	assert.Nil(t, output.CreatedDate)
}

func TestShowUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "v2-value",
		Version: domain.Version{ID: "v2-id"},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret~1")})
	require.NoError(t, err)
	assert.Equal(t, "v2-id", output.VersionID)
	assert.Equal(t, "v2-value", output.Value)
}

func TestShowUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "secret-value",
		Version: domain.Version{ID: "abc123"},
		Tags: []domain.Tag{
			{Key: "env", Value: "prod"},
			{Key: "team", Value: "backend"},
		},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Spec: mustParseSpec(t, "my-secret")})
	require.NoError(t, err)
	require.Len(t, output.Tags, 2)
	assert.Equal(t, "env", output.Tags[0].Key)
	assert.Equal(t, "prod", output.Tags[0].Value)
	assert.Equal(t, "team", output.Tags[1].Key)
	assert.Equal(t, "backend", output.Tags[1].Value)
}
