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

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "current-value"}, nil
		},
	}

	uc := &param.DeleteUseCase{Store: store}

	value, err := uc.GetCurrentValue(t.Context(), "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestDeleteUseCase_GetCurrentValue_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	uc := &param.DeleteUseCase{Store: store}

	// A non-existent parameter yields an empty preview with no error.
	value, err := uc.GetCurrentValue(t.Context(), "/app/not-exists")
	require.NoError(t, err)
	assert.Empty(t, value)
}

// TestDeleteUseCase_GetCurrentValue_Error verifies a non-not-found read failure
// is propagated (not swallowed).
func TestDeleteUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errAWS
		},
	}

	uc := &param.DeleteUseCase{Store: store}

	_, err := uc.GetCurrentValue(t.Context(), "/app/config")
	require.Error(t, err)
	require.ErrorIs(t, err, errAWS)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string) error {
			assert.Equal(t, "/app/to-delete", name)

			return nil
		},
	}

	uc := &param.DeleteUseCase{Store: store}

	output, err := uc.Execute(t.Context(), param.DeleteInput{Name: "/app/to-delete"})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)
}

func TestDeleteUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, _ string) error {
			return errDeleteFailed
		},
	}

	uc := &param.DeleteUseCase{Store: store}

	_, err := uc.Execute(t.Context(), param.DeleteInput{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete parameter")
}
