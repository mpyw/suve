// White-box tests of create.go.
//declscope:namespace create

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

func TestCreateRun_WriteOptions(t *testing.T) {
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

		r := &createRunner{useCase: &param.CreateUseCase{Writer: store}, stdout: &buf, stderr: &errBuf}
		err := r.run(t.Context(), createOptions{
			name:      "/app/param",
			value:     "v",
			paramType: "String",
			paramOpts: writeOptionFlags{
				tier:           "Advanced",
				dataType:       "text",
				allowedPattern: "^a",
				policies:       "[]",
			},
		})
		require.NoError(t, err)
		require.Len(t, gotOpts, 4)
		assert.Contains(t, gotOpts, parameterstore.Tier{Value: "Advanced"})
		assert.Contains(t, gotOpts, parameterstore.DataType{Value: "text"})
		assert.Contains(t, gotOpts, parameterstore.AllowedPattern{Value: "^a"})
		assert.Contains(t, gotOpts, parameterstore.Policies{JSON: "[]"})
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

		r := &createRunner{useCase: &param.CreateUseCase{Writer: store}, stdout: &buf, stderr: &errBuf}
		err := r.run(t.Context(), createOptions{name: "/app/param", value: "v", paramType: "String"})
		require.NoError(t, err)
		assert.Empty(t, gotOpts)
	})
}

func TestCreateRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    createOptions
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "create parameter",
			opts: createOptions{
				name:      "/app/param",
				value:     "test-value",
				paramType: "SecureString",
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
			opts: createOptions{
				name:        "/app/param",
				value:       "test-value",
				paramType:   "String",
				description: "Test description",
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
			opts:    createOptions{name: "/app/param", value: "test-value", paramType: "String"},
			wantErr: "failed to create parameter",
			store: &providermock.Store{
				CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
					return domain.Version{}, provider.ErrAlreadyExists
				},
			},
		},
		{
			name:    "create AWS error",
			opts:    createOptions{name: "/app/param", value: "test-value", paramType: "String"},
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

			r := &createRunner{
				useCase: &param.CreateUseCase{Writer: tt.store, ItemNoun: "parameter"},
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
