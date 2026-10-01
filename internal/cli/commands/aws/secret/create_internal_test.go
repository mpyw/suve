// White-box tests of create.go.
//declscope:namespace create

package secret

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

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
			name: "create secret",
			opts: createOptions{name: "my-secret", value: "secret-value"},
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
			opts: createOptions{name: "my-secret", value: "secret-value", description: "Test description"},
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
			opts:    createOptions{name: "my-secret", value: "secret-value"},
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

			r := &createRunner{
				useCase: &secret.CreateUseCase{Writer: tt.store},
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
