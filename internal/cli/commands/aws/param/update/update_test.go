package update_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramopts"
	"github.com/mpyw/suve/internal/cli/commands/aws/param/update"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
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
		err := app.Run(t.Context(), []string{"suve", "param", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	// The value is now optional (--value-stdin / editor fallback), but a
	// positional value cannot be combined with --value-stdin.
	t.Run("positional value with --value-stdin conflicts", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "/app/param", "value", "--value-stdin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine a positional value with --value-stdin")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})

	t.Run("invalid tier value", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "--yes", "--tier", "Bogus", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --tier")
	})

	// A typo/wrong-case --type must be rejected, not silently stored as plaintext.
	t.Run("invalid type value", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "--yes", "--type", "securestring", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --type")
	})
}

func TestRun_WriteOptions(t *testing.T) {
	t.Parallel()

	existsGet := func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
		return &domain.Entry{Name: name, Value: "old-value"}, nil
	}

	t.Run("set flags produce options", func(t *testing.T) {
		t.Parallel()

		var gotOpts []provider.WriteOption

		store := &providermock.Store{
			GetFunc: existsGet,
			PutFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, opts ...provider.WriteOption,
			) (domain.Version, error) {
				gotOpts = opts

				return domain.Version{ID: "2"}, nil
			},
		}

		var buf, errBuf bytes.Buffer

		r := &update.Runner{UseCase: &param.UpdateUseCase{Store: store}, Stdout: &buf, Stderr: &errBuf}
		err := r.Run(t.Context(), update.Options{
			Name:  "/app/param",
			Value: "v",
			Type:  "String",
			ParamOpts: paramopts.Values{
				Tier:     "Intelligent-Tiering",
				DataType: "text",
			},
		})
		require.NoError(t, err)
		require.Len(t, gotOpts, 2)
		assert.Contains(t, gotOpts, awsparam.Tier{Value: "Intelligent-Tiering"})
		assert.Contains(t, gotOpts, awsparam.DataType{Value: "text"})
	})

	t.Run("unset flags produce no options", func(t *testing.T) {
		t.Parallel()

		var gotOpts []provider.WriteOption

		store := &providermock.Store{
			GetFunc: existsGet,
			PutFunc: func(
				_ context.Context, _, _ string, _ domain.ValueType, _ string, opts ...provider.WriteOption,
			) (domain.Version, error) {
				gotOpts = opts

				return domain.Version{ID: "2"}, nil
			},
		}

		var buf, errBuf bytes.Buffer

		r := &update.Runner{UseCase: &param.UpdateUseCase{Store: store}, Stdout: &buf, Stderr: &errBuf}
		err := r.Run(t.Context(), update.Options{Name: "/app/param", Value: "v", Type: "String"})
		require.NoError(t, err)
		assert.Empty(t, gotOpts)
	})
}

// TestRun_PreserveType verifies that a value-only update (PreserveType, no
// --type/--secure) reuses the existing parameter's type, so an existing
// SecureString is not silently rewritten as String.
func TestRun_PreserveType(t *testing.T) {
	t.Parallel()

	var gotType domain.ValueType

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "old-value", Type: domain.ValueTypeSecret}, nil
		},
		PutFunc: func(_ context.Context, _, _ string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			gotType = vt

			return domain.Version{ID: "2"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &update.Runner{UseCase: &param.UpdateUseCase{Store: store}, Stdout: &buf, Stderr: &errBuf}
	err := r.Run(t.Context(), update.Options{Name: "/app/secret", Value: "v", PreserveType: true})
	require.NoError(t, err)
	assert.Equal(t, domain.ValueTypeSecret, gotType)
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
				PutFunc: func(_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
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
				PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption) (domain.Version, error) {
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
				PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
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
