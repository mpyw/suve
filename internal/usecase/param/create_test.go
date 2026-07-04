package param_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCreateUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			assert.Equal(t, "/app/new", name)
			assert.Equal(t, "new-value", value)
			assert.Equal(t, domain.ValueTypePlaintext, vt)

			return domain.Version{ID: "1"}, nil
		},
	}

	uc := &param.CreateUseCase{Writer: store}

	output, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/new",
		Value: "new-value",
		Type:  domain.ValueTypePlaintext,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
}

func TestCreateUseCase_Execute_WithDescription(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption) (domain.Version, error) {
			assert.Equal(t, "my description", description)

			return domain.Version{ID: "1"}, nil
		},
	}

	uc := &param.CreateUseCase{Writer: store}

	output, err := uc.Execute(t.Context(), param.CreateInput{
		Name:        "/app/new",
		Value:       "new-value",
		Type:        domain.ValueTypePlaintext,
		Description: "my description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
}

// TestCreateUseCase_Execute_AlreadyExists verifies the create-only behavior:
// when the provider reports the entry already exists, create surfaces the error
// (and never overwrites).
func TestCreateUseCase_Execute_AlreadyExists(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			return domain.Version{}, provider.ErrAlreadyExists
		},
	}

	uc := &param.CreateUseCase{Writer: store}

	_, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/existing",
		Value: "value",
		Type:  domain.ValueTypePlaintext,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parameter")
	require.ErrorIs(t, err, provider.ErrAlreadyExists)
}

func TestCreateUseCase_Execute_CreateError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			return domain.Version{}, errPutFailed
		},
	}

	uc := &param.CreateUseCase{Writer: store}

	_, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  domain.ValueTypePlaintext,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parameter")
}
