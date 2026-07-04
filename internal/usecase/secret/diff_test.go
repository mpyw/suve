package secret_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestDiffUseCase_Execute(t *testing.T) {
	t.Parallel()

	entries := map[string]*domain.Entry{
		"":       {Name: "my-secret", Value: "new-value", Version: domain.Version{ID: "v2-id"}},
		"#v1-id": {Name: "my-secret", Value: "old-value", Version: domain.Version{ID: "v1-id"}},
	}

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			// Encode the spec into the ref id so Get can pick the right entry.
			return provider.NewVersionRef(spec), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			return entries[ref.ID()], nil
		},
	}

	uc := &secret.DiffUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: mustParseSpec(t, "my-secret#v1-id"),
		Spec2: mustParseSpec(t, "my-secret"),
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.OldName)
	assert.Equal(t, "v1-id", output.OldVersionID)
	assert.Equal(t, "old-value", output.OldValue)
	assert.Equal(t, "my-secret", output.NewName)
	assert.Equal(t, "v2-id", output.NewVersionID)
	assert.Equal(t, "new-value", output.NewValue)
}

func TestDiffUseCase_Execute_ResolveError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, assert.AnError
		},
	}

	uc := &secret.DiffUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: mustParseSpec(t, "my-secret"),
		Spec2: mustParseSpec(t, "my-secret~1"),
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, assert.AnError
		},
	}

	uc := &secret.DiffUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), secret.DiffInput{
		Spec1: mustParseSpec(t, "my-secret"),
		Spec2: mustParseSpec(t, "my-secret~1"),
	})
	assert.Error(t, err)
}
