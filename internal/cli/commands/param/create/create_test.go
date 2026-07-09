package create_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/cli/commands/param/create"
	"github.com/mpyw/suve/internal/cli/commands/param/paramopts"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})

	t.Run("invalid tier value", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "--tier", "Bogus", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --tier")
	})

	// A typo/wrong-case --type must be rejected, not silently stored as plaintext.
	t.Run("invalid type value", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "--type", "securestring", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --type")
	})
}

func TestRun_WriteOptions(t *testing.T) {
	t.Parallel()

	t.Run("set flags produce options", func(t *testing.T) {
		t.Parallel()

		var gotOpts []provider.WriteOption

		store := &providermock.Store{
			CreateFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, opts ...provider.WriteOption,
			) (domain.Version, error) {
				gotOpts = opts

				return domain.Version{ID: "1"}, nil
			},
		}

		var buf, errBuf bytes.Buffer

		r := &create.Runner{UseCase: &param.CreateUseCase{Writer: store}, Stdout: &buf, Stderr: &errBuf}
		err := r.Run(t.Context(), create.Options{
			Name:  "/app/param",
			Value: "v",
			Type:  "String",
			ParamOpts: paramopts.Values{
				Tier:           "Advanced",
				DataType:       "text",
				AllowedPattern: "^a",
				Policies:       "[]",
			},
		})
		require.NoError(t, err)
		require.Len(t, gotOpts, 4)
		assert.Contains(t, gotOpts, awsparam.Tier{Value: "Advanced"})
		assert.Contains(t, gotOpts, awsparam.DataType{Value: "text"})
		assert.Contains(t, gotOpts, awsparam.AllowedPattern{Value: "^a"})
		assert.Contains(t, gotOpts, awsparam.Policies{JSON: "[]"})
	})

	t.Run("unset flags produce no options", func(t *testing.T) {
		t.Parallel()

		var gotOpts []provider.WriteOption

		store := &providermock.Store{
			CreateFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, opts ...provider.WriteOption,
			) (domain.Version, error) {
				gotOpts = opts

				return domain.Version{ID: "1"}, nil
			},
		}

		var buf, errBuf bytes.Buffer

		r := &create.Runner{UseCase: &param.CreateUseCase{Writer: store}, Stdout: &buf, Stderr: &errBuf}
		err := r.Run(t.Context(), create.Options{Name: "/app/param", Value: "v", Type: "String"})
		require.NoError(t, err)
		assert.Empty(t, gotOpts)
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
			name: "create parameter",
			opts: create.Options{
				Name:  "/app/param",
				Value: "test-value",
				Type:  "SecureString",
			},
			store: &providermock.Store{
				CreateFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
					assert.Equal(t, "/app/param", name)
					assert.Equal(t, "test-value", value)
					assert.Equal(t, domain.ValueTypeSecret, vt)

					return domain.Version{ID: "1"}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Created parameter")
				assert.Contains(t, output, "/app/param")
				assert.Contains(t, output, "version: 1")
			},
		},
		{
			name: "create with description",
			opts: create.Options{
				Name:        "/app/param",
				Value:       "test-value",
				Type:        "String",
				Description: "Test description",
			},
			store: &providermock.Store{
				CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption) (domain.Version, error) {
					assert.Equal(t, "Test description", description)

					return domain.Version{ID: "1"}, nil
				},
			},
		},
		{
			// Genuine already-exists behavior: the provider reports the entry
			// exists and create surfaces the error (never overwrites).
			name:    "create already exists error",
			opts:    create.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "failed to create parameter",
			store: &providermock.Store{
				CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
					return domain.Version{}, provider.ErrAlreadyExists
				},
			},
		},
		{
			name:    "create AWS error",
			opts:    create.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "failed to create parameter",
			store: &providermock.Store{
				CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
					return domain.Version{}, assert.AnError
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &create.Runner{
				UseCase: &param.CreateUseCase{Writer: tt.store},
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
