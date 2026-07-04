package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestUpdateUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "current-value"}, nil
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	value, err := uc.GetCurrentValue(t.Context(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestUpdateUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errors.New("aws error")
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	_, err := uc.GetCurrentValue(t.Context(), "my-secret")
	assert.Error(t, err)
}

func TestUpdateUseCase_Execute_UpdateValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "old-value"}, nil
		},
		PutFunc: func(
			_ context.Context, name, value string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "my-secret", name)
			assert.Equal(t, "new-value", value)
			assert.Equal(t, domain.ValueTypeSecret, valueType)

			return domain.Version{ID: "new-version-id"}, nil
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	output, err := uc.Execute(t.Context(), secret.UpdateInput{Name: "my-secret", Value: "new-value"})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "new-version-id", output.VersionID)
}

// TestUpdateUseCase_Execute_UpdateValueAndDescription is a genuine
// anti-regression test: the description must be forwarded to the provider (the
// adapter updates it on the existing secret via UpdateSecret).
func TestUpdateUseCase_Execute_UpdateValueAndDescription(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "old-value"}, nil
		},
		PutFunc: func(
			_ context.Context, _, value string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "new-value", value)
			assert.Equal(t, "new description", description)

			return domain.Version{ID: "new-version-id"}, nil
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	output, err := uc.Execute(t.Context(), secret.UpdateInput{
		Name:        "my-secret",
		Value:       "new-value",
		Description: "new description",
	})
	require.NoError(t, err)
	assert.Equal(t, "new-version-id", output.VersionID)
}

func TestUpdateUseCase_Execute_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	_, err := uc.Execute(t.Context(), secret.UpdateInput{Name: "missing", Value: "v"})
	require.Error(t, err)
	assert.ErrorIs(t, err, secret.ErrSecretNotFound)
}

func TestUpdateUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errors.New("access denied")
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	_, err := uc.Execute(t.Context(), secret.UpdateInput{Name: "my-secret", Value: "v"})
	require.Error(t, err)
	// A non-not-found read error is propagated, never treated as "does not exist".
	assert.NotErrorIs(t, err, secret.ErrSecretNotFound)
}

func TestUpdateUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "old-value"}, nil
		},
		PutFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			return domain.Version{}, errors.New("put failed")
		},
	}

	uc := &secret.UpdateUseCase{Store: store}

	_, err := uc.Execute(t.Context(), secret.UpdateInput{Name: "my-secret", Value: "new-value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update secret")
}
