package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestRestoreUseCase_Execute(t *testing.T) {
	t.Parallel()

	var gotName string

	store := &providermock.Store{
		RestoreFunc: func(_ context.Context, name string) error {
			gotName = name

			return nil
		},
	}

	uc := &secret.RestoreUseCase{Restorer: store}

	output, err := uc.Execute(t.Context(), secret.RestoreInput{Name: "my-secret"})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "my-secret", gotName)
}

func TestRestoreUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		RestoreFunc: func(_ context.Context, _ string) error {
			return errors.New("restore failed")
		},
	}

	uc := &secret.RestoreUseCase{Restorer: store}

	_, err := uc.Execute(t.Context(), secret.RestoreInput{Name: "my-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to restore secret")
}
