package create_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/aws/secret/create"
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
		err := app.Run(t.Context(), []string{"suve", "secret", "create"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	// The value is now optional (--value-stdin / editor fallback), but a
	// positional value cannot be combined with --value-stdin.
	t.Run("positional value with --value-stdin conflicts", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "create", "my-secret", "value", "--value-stdin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine a positional value with --value-stdin")
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    create.Options
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "create secret",
			opts: create.Options{Name: "my-secret", Value: "secret-value"},
			store: &providermock.Store{
				CreateFunc: func(
					_ context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
				) (domain.Version, error) {
					assert.Equal(t, "my-secret", name)
					assert.Equal(t, "secret-value", value)

					return domain.Version{ID: "abc123"}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Created secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "create with description",
			opts: create.Options{Name: "my-secret", Value: "secret-value", Description: "Test description"},
			store: &providermock.Store{
				CreateFunc: func(
					_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
				) (domain.Version, error) {
					assert.Equal(t, "Test description", description)

					return domain.Version{ID: "abc123"}, nil
				},
			},
		},
		{
			name:    "error from AWS",
			opts:    create.Options{Name: "my-secret", Value: "secret-value"},
			wantErr: "failed to create secret",
			store: &providermock.Store{
				CreateFunc: func(
					_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
				) (domain.Version, error) {
					return domain.Version{}, errors.New("AWS error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &create.Runner{
				UseCase: &secret.CreateUseCase{Writer: tt.store},
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
