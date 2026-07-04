package secret_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCreateUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, value string, valueType domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "my-secret", name)
			assert.Equal(t, "secret-value", value)
			assert.Equal(t, domain.ValueTypeSecret, valueType)

			return domain.Version{ID: "abc123"}, nil
		},
	}

	uc := &secret.CreateUseCase{Writer: store}

	output, err := uc.Execute(t.Context(), secret.CreateInput{
		Name:  "my-secret",
		Value: "secret-value",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "abc123", output.VersionID)
}

func TestCreateUseCase_Execute_WithDescription(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "my description", description)

			return domain.Version{ID: "abc123"}, nil
		},
	}

	uc := &secret.CreateUseCase{Writer: store}

	output, err := uc.Execute(t.Context(), secret.CreateInput{
		Name:        "my-secret",
		Value:       "secret-value",
		Description: "my description",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
}

func TestCreateUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			return domain.Version{}, errors.New("aws error")
		},
	}

	uc := &secret.CreateUseCase{Writer: store}

	_, err := uc.Execute(t.Context(), secret.CreateInput{Name: "my-secret", Value: "secret-value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create secret")
}

// TestCreateUseCase_Execute_AlreadyExists is the genuine anti-regression test:
// a create against an existing secret must surface provider.ErrAlreadyExists
// (create-only semantics), never silently overwrite.
func TestCreateUseCase_Execute_AlreadyExists(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
		},
	}

	uc := &secret.CreateUseCase{Writer: store}

	_, err := uc.Execute(t.Context(), secret.CreateInput{Name: "my-secret", Value: "secret-value"})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrAlreadyExists)
}
