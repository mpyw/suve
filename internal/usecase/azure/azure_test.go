package azure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/azure"
)

func TestShowUseCase(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Equal(t, "#abc", spec)

			return provider.NewVersionRef("abc"), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			assert.Equal(t, "abc", ref.ID())

			return &domain.Entry{
				Name:    name,
				Value:   "hello",
				Version: domain.Version{ID: "abc", Label: "enabled"},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	uc := &azure.ShowUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.ShowInput{Name: "my-secret", Suffix: "#abc"})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", out.Name)
	assert.Equal(t, "hello", out.Value)
	assert.Equal(t, "abc", out.Version)
	assert.Equal(t, "enabled", out.State)
	assert.Equal(t, []azure.ShowTag{{Key: "env", Value: "prod"}}, out.Tags)
}

// TestLogUseCase_HistoryErrorPropagates is the acid-test at the use-case layer:
// an unversioned store returns an error from History, and the log use case
// propagates it (rather than crashing or returning an empty history).
func TestLogUseCase_HistoryErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("Azure App Configuration does not support versions")

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return nil, sentinel
		},
	}

	uc := &azure.LogUseCase{Reader: store}
	_, err := uc.Execute(t.Context(), azure.LogInput{Name: "my-key"})
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
}

func TestDiffUseCase(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, name, _ string) (provider.VersionRef, error) {
			return provider.NewVersionRef(name), nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "val-" + name}, nil
		},
	}

	uc := &azure.DiffUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.DiffInput{Name1: "key-a", Name2: "key-b"})
	require.NoError(t, err)
	assert.Equal(t, "key-a", out.OldName)
	assert.Equal(t, "key-b", out.NewName)
	assert.Equal(t, "val-key-a", out.OldValue)
	assert.Equal(t, "val-key-b", out.NewValue)
}

func TestUpdateUseCase_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	uc := &azure.UpdateUseCase{Store: store}
	_, err := uc.Execute(t.Context(), azure.UpdateInput{Name: "missing", Value: "v", ValueType: domain.ValueTypePlaintext})
	require.ErrorIs(t, err, azure.ErrEntryNotFound)
}
