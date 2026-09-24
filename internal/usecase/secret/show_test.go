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

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := showStore(&domain.Entry{
		Name:  "my-secret",
		Value: "secret-value",
		Type:  domain.ValueTypeSecret,
		// Multiple staging labels (unsorted) must all surface, deterministically
		// sorted for stable output (#419).
		Version: domain.Version{ID: "abc123", Labels: []string{"AWSCURRENT", "custom"}, Created: &now},
		Extra:   []domain.Field{{Label: "ARN", Value: "arn:aws:secretsmanager:us-east-1:123:secret:my-secret"}},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret"})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "secret-value", output.Value)
	assert.Equal(t, "abc123", output.Version)
	assert.Equal(t, []string{"AWSCURRENT", "custom"}, output.Labels)
	// AWS Secrets Manager carries staging labels, not a per-version state (#419).
	assert.Empty(t, output.State)
	assert.NotNil(t, output.CreatedDate)
	// The provider's display-only metadata (the ARN here) passes through verbatim.
	assert.Equal(t, []domain.Field{{Label: "ARN", Value: "arn:aws:secretsmanager:us-east-1:123:secret:my-secret"}}, output.Extra)
}

// TestShowUseCase_Execute_State asserts that a GCloud/Key Vault-style version
// (per-version State set, no staging labels) surfaces State and leaves
// Labels empty — the two concepts must not be conflated (#419).
func TestShowUseCase_Execute_State(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "secret-value",
		Type:    domain.ValueTypeSecret,
		Version: domain.Version{ID: "2", State: "enabled"},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret"})
	require.NoError(t, err)
	assert.Equal(t, "enabled", output.State)
	assert.Empty(t, output.Labels)
}

func TestShowUseCase_Execute_WithVersionID(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "old-value",
		Version: domain.Version{ID: "old-version-id", Labels: []string{"AWSPREVIOUS"}},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret", Suffix: "#old-version-id"})
	require.NoError(t, err)
	assert.Equal(t, "old-value", output.Value)
	assert.Equal(t, "old-version-id", output.Version)
}

func TestShowUseCase_Execute_WithLabel(t *testing.T) {
	t.Parallel()

	store := showStore(&domain.Entry{
		Name:    "my-secret",
		Value:   "current-value",
		Version: domain.Version{ID: "current-id", Labels: []string{"AWSCURRENT"}},
	})

	uc := &secret.ShowUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret", Suffix: ":AWSCURRENT"})
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

	_, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret"})
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

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret"})
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

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret", Suffix: "~1"})
	require.NoError(t, err)
	assert.Equal(t, "v2-id", output.Version)
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

	output, err := uc.Execute(t.Context(), secret.ShowInput{Name: "my-secret"})
	require.NoError(t, err)
	require.Len(t, output.Tags, 2)
	assert.Equal(t, "env", output.Tags[0].Key)
	assert.Equal(t, "prod", output.Tags[0].Value)
	assert.Equal(t, "team", output.Tags[1].Key)
	assert.Equal(t, "backend", output.Tags[1].Value)
}
