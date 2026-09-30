// White-box tests of delete.go.
//declscope:namespace delete

package param

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
)

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "current-value"}, nil
		},
	}

	uc := &DeleteUseCase{Store: store}

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

	uc := &DeleteUseCase{Store: store}

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
			return nil, errHelperAWS
		},
	}

	uc := &DeleteUseCase{Store: store}

	_, err := uc.GetCurrentValue(t.Context(), "/app/config")
	require.Error(t, err)
	require.ErrorIs(t, err, errHelperAWS)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
			assert.Equal(t, "/app/to-delete", name)

			return nil
		},
	}

	uc := &DeleteUseCase{Store: store}

	output, err := uc.Execute(t.Context(), DeleteInput{Name: "/app/to-delete"})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)
}

func TestDeleteUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
			return errHelperDeleteFailed
		},
	}

	uc := &DeleteUseCase{Store: store, ItemNoun: "setting"}

	_, err := uc.Execute(t.Context(), DeleteInput{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete setting")
}
