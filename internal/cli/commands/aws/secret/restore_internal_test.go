// White-box tests of restore.go.
//declscope:namespace restore

package secret

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestRestoreRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    restoreOptions
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "restore secret",
			opts: restoreOptions{name: "my-secret"},
			store: &providermock.Store{
				RestoreFunc: func(_ context.Context, name string) error {
					assert.Equal(t, "my-secret", name)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Restored secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "error from AWS",
			opts: restoreOptions{name: "my-secret"},
			store: &providermock.Store{
				RestoreFunc: func(_ context.Context, _ string) error {
					return errors.New("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &restoreRunner{
				useCase: &secret.RestoreUseCase{Restorer: tt.store},
				stdout:  &buf,
				stderr:  &errBuf,
			}
			err := r.run(t.Context(), tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
