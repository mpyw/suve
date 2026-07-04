package param_test

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
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestUpdateUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "current-value"}, nil
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	value, err := uc.GetCurrentValue(t.Context(), "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestUpdateUseCase_GetCurrentValue_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, fmt.Errorf("%w: /app/missing", provider.ErrNotFound)
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	value, err := uc.GetCurrentValue(t.Context(), "/app/missing")
	require.NoError(t, err)
	assert.Empty(t, value)
}

func TestUpdateUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errors.New("aws error")
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	_, err := uc.GetCurrentValue(t.Context(), "/app/config")
	assert.Error(t, err)
}

func TestUpdateUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name}, nil
		},
		PutFunc: func(_ context.Context, name, value string, vt domain.ValueType, description string, _ ...provider.WriteOption) (domain.Version, error) {
			assert.Equal(t, "/app/config", name)
			assert.Equal(t, "updated-value", value)
			assert.Equal(t, domain.ValueTypePlaintext, vt)
			assert.Equal(t, "updated description", description)

			return domain.Version{ID: "5"}, nil
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	output, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:        "/app/config",
		Value:       "updated-value",
		Type:        domain.ValueTypePlaintext,
		Description: "updated description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, int64(5), output.Version)
}

// TestUpdateUseCase_Execute_NotFound verifies a missing parameter is reported
// as not-found (only provider.ErrNotFound triggers this path).
func TestUpdateUseCase_Execute_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	_, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:  "/app/not-exists",
		Value: "value",
		Type:  domain.ValueTypePlaintext,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter not found")
}

// TestUpdateUseCase_Execute_ReadError verifies a non-not-found read failure is
// propagated (NOT swallowed as "does not exist").
func TestUpdateUseCase_Execute_ReadError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errAWS
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	_, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  domain.ValueTypePlaintext,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, errAWS)
	assert.NotContains(t, err.Error(), "parameter not found")
}

func TestUpdateUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name}, nil
		},
		PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			return domain.Version{}, errPutFailed
		},
	}

	uc := &param.UpdateUseCase{Store: store}

	_, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  domain.ValueTypePlaintext,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update parameter")
}
