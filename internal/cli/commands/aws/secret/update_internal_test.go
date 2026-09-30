// White-box tests of update.go.
//declscope:namespace update

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

func TestUpdateRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    updateOptions
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update secret",
			opts: updateOptions{name: "my-secret", value: "new-value"},
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
			opts: updateOptions{name: "my-secret", value: "new-value", description: "updated description"},
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
			opts:    updateOptions{name: "my-secret", value: "new-value"},
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
			opts:    updateOptions{name: "my-secret", value: "new-value"},
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

			r := &updateRunner{
				useCase: &secret.UpdateUseCase{Store: tt.store},
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
