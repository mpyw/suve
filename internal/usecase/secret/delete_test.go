package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "current-value"}, nil
		},
	}

	uc := &secret.DeleteUseCase{Store: store}

	value, err := uc.GetCurrentValue(t.Context(), "my-secret")
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

	uc := &secret.DeleteUseCase{Store: store}

	value, err := uc.GetCurrentValue(t.Context(), "not-exists")
	require.NoError(t, err)
	assert.Empty(t, value)
}

func TestDeleteUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errors.New("aws error")
		},
	}

	uc := &secret.DeleteUseCase{Store: store}

	_, err := uc.GetCurrentValue(t.Context(), "my-secret")
	assert.Error(t, err)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	var gotName string

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, opts ...provider.DeleteOption) error {
			gotName = name

			assert.Empty(t, opts, "no options for a default delete")

			return nil
		},
	}

	uc := &secret.DeleteUseCase{Store: store}

	output, err := uc.Execute(t.Context(), secret.DeleteInput{Name: "my-secret"})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "my-secret", gotName)
}

// TestDeleteUseCase_Execute_ForwardsForceDelete is a genuine anti-regression
// test: the ForceDelete option must be forwarded verbatim to the provider.
func TestDeleteUseCase_Execute_ForwardsForceDelete(t *testing.T) {
	t.Parallel()

	var gotOpts []provider.DeleteOption

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
			gotOpts = opts

			return nil
		},
	}

	uc := &secret.DeleteUseCase{Store: store}

	_, err := uc.Execute(t.Context(), secret.DeleteInput{
		Name:    "my-secret",
		Options: []provider.DeleteOption{awssecret.ForceDelete{}},
	})
	require.NoError(t, err)
	require.Len(t, gotOpts, 1)
	assert.IsType(t, awssecret.ForceDelete{}, gotOpts[0])
}

// TestDeleteUseCase_Execute_ForwardsRecoveryWindow is a genuine anti-regression
// test: the RecoveryWindow option (with its Days) must be forwarded verbatim.
func TestDeleteUseCase_Execute_ForwardsRecoveryWindow(t *testing.T) {
	t.Parallel()

	var gotOpts []provider.DeleteOption

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
			gotOpts = opts

			return nil
		},
	}

	uc := &secret.DeleteUseCase{Store: store}

	_, err := uc.Execute(t.Context(), secret.DeleteInput{
		Name:    "my-secret",
		Options: []provider.DeleteOption{awssecret.RecoveryWindow{Days: 7}},
	})
	require.NoError(t, err)
	require.Len(t, gotOpts, 1)
	rw, ok := gotOpts[0].(awssecret.RecoveryWindow)
	require.True(t, ok, "expected a RecoveryWindow option")
	assert.Equal(t, int64(7), rw.Days)
}

func TestDeleteUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
			return errors.New("delete failed")
		},
	}

	uc := &secret.DeleteUseCase{Store: store}

	_, err := uc.Execute(t.Context(), secret.DeleteInput{Name: "my-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete secret")
}
