// White-box tests of delete.go.
//declscope:namespace delete

package param

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestDeleteRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    deleteOptions
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete parameter",
			opts: deleteOptions{name: "/app/param"},
			store: &providermock.Store{
				DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Deleted")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name: "error from AWS",
			opts: deleteOptions{name: "/app/param"},
			store: &providermock.Store{
				DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
					return assert.AnError
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &deleteRunner{
				useCase: &param.DeleteUseCase{Store: tt.store, ItemNoun: "parameter"},
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
