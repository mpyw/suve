package update_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/update"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	// Default GetFunc simulates an existing parameter.
	existsGet := func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
		return &domain.Entry{Name: name, Value: "old-value"}, nil
	}

	tests := []struct {
		name    string
		opts    update.Options
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update parameter",
			opts: update.Options{
				Name:  "/app/param",
				Value: "test-value",
				Type:  "SecureString",
			},
			store: &providermock.Store{
				GetFunc: existsGet,
				PutFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string) (domain.Version, error) {
					assert.Equal(t, "/app/param", name)
					assert.Equal(t, "test-value", value)
					assert.Equal(t, domain.ValueTypeSecret, vt)

					return domain.Version{ID: "2"}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Updated parameter")
				assert.Contains(t, output, "/app/param")
				assert.Contains(t, output, "version: 2")
			},
		},
		{
			name: "update with description",
			opts: update.Options{
				Name:        "/app/param",
				Value:       "test-value",
				Type:        "String",
				Description: "Test description",
			},
			store: &providermock.Store{
				GetFunc: existsGet,
				PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, description string) (domain.Version, error) {
					assert.Equal(t, "Test description", description)

					return domain.Version{ID: "2"}, nil
				},
			},
		},
		{
			name:    "update not found error",
			opts:    update.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "parameter not found",
			store: &providermock.Store{
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return nil, provider.ErrNotFound
				},
			},
		},
		{
			name:    "update AWS error",
			opts:    update.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "failed to update parameter",
			store: &providermock.Store{
				GetFunc: existsGet,
				PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string) (domain.Version, error) {
					return domain.Version{}, assert.AnError
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &update.Runner{
				UseCase: &param.UpdateUseCase{Store: tt.store},
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
