// White-box tests of update.go.
//declscope:namespace update

package param

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/parameterstore"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestUpdateRun_WriteOptions(t *testing.T) {
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

		r := &updateRunner{useCase: &param.UpdateUseCase{Store: store}, stdout: &buf, stderr: &errBuf}
		err := r.run(t.Context(), updateOptions{
			name:      "/app/param",
			value:     "v",
			paramType: "String",
			paramOpts: writeOptionFlags{
				tier:     "Intelligent-Tiering",
				dataType: "text",
			},
		})
		require.NoError(t, err)
		require.Len(t, gotOpts, 2)
		assert.Contains(t, gotOpts, parameterstore.Tier{Value: "Intelligent-Tiering"})
		assert.Contains(t, gotOpts, parameterstore.DataType{Value: "text"})
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

		r := &updateRunner{useCase: &param.UpdateUseCase{Store: store}, stdout: &buf, stderr: &errBuf}
		err := r.run(t.Context(), updateOptions{name: "/app/param", value: "v", paramType: "String"})
		require.NoError(t, err)
		assert.Empty(t, gotOpts)
	})
}

// TestRun_PreserveType verifies that a value-only update (PreserveType, no
// --type/--secure) reuses the existing parameter's type, so an existing
// SecureString is not silently rewritten as String.
func TestUpdateRun_PreserveType(t *testing.T) {
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

	r := &updateRunner{useCase: &param.UpdateUseCase{Store: store}, stdout: &buf, stderr: &errBuf}
	err := r.run(t.Context(), updateOptions{name: "/app/secret", value: "v", preserveType: true})
	require.NoError(t, err)
	assert.Equal(t, domain.ValueTypeSecret, gotType)
}

func TestUpdateRun(t *testing.T) {
	t.Parallel()

	// Default GetFunc simulates an existing parameter.
	existsGet := func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
		return &domain.Entry{Name: name, Value: "old-value"}, nil
	}

	tests := []struct {
		name    string
		opts    updateOptions
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update parameter",
			opts: updateOptions{
				name:      "/app/param",
				value:     "test-value",
				paramType: "SecureString",
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
			opts: updateOptions{
				name:        "/app/param",
				value:       "test-value",
				paramType:   "String",
				description: "Test description",
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
			opts:    updateOptions{name: "/app/param", value: "test-value", paramType: "String"},
			wantErr: "parameter not found",
			store: &providermock.Store{
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return nil, provider.ErrNotFound
				},
			},
		},
		{
			name:    "update AWS error",
			opts:    updateOptions{name: "/app/param", value: "test-value", paramType: "String"},
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

			r := &updateRunner{
				useCase: &param.UpdateUseCase{Store: tt.store, ItemNoun: "parameter"},
				stdout:  &buf,
				stderr:  &errBuf,
			}
			err := r.run(t.Context(), tt.opts)

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
