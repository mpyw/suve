package update_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/aws/secret/update"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	// The value is now optional (--value-stdin / editor fallback), but a
	// positional value cannot be combined with --value-stdin.
	t.Run("positional value with --value-stdin conflicts", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update", "my-secret", "value", "--value-stdin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine a positional value with --value-stdin")
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    update.Options
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update secret",
			opts: update.Options{Name: "my-secret", Value: "new-value"},
			store: &providermock.Store{
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return &domain.Entry{Value: "old-value"}, nil
				},
				PutFunc: func(
					_ context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
				) (domain.Version, error) {
					assert.Equal(t, "my-secret", name)
					assert.Equal(t, "new-value", value)

					return domain.Version{ID: "new-version-id"}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Updated secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "update secret with description",
			opts: update.Options{Name: "my-secret", Value: "new-value", Description: "updated description"},
			store: &providermock.Store{
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return &domain.Entry{Value: "old-value"}, nil
				},
				PutFunc: func(
					_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
				) (domain.Version, error) {
					assert.Equal(t, "updated description", description)

					return domain.Version{ID: "new-version-id"}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Updated secret")
			},
		},
		{
			name:    "put secret value error",
			opts:    update.Options{Name: "my-secret", Value: "new-value"},
			wantErr: "failed to update secret",
			store: &providermock.Store{
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return &domain.Entry{Value: "old-value"}, nil
				},
				PutFunc: func(
					_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
				) (domain.Version, error) {
					return domain.Version{}, errors.New("AWS error")
				},
			},
		},
		{
			name:    "secret not found",
			opts:    update.Options{Name: "my-secret", Value: "new-value"},
			wantErr: "secret not found",
			store: &providermock.Store{
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return nil, provider.ErrNotFound
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &update.Runner{
				UseCase: &secret.UpdateUseCase{Store: tt.store},
				Stdout:  &buf,
				Stderr:  &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
