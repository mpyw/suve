package generic_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/provider/providermock"
)

func TestRunUntag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		noun    string
		resName string
		keys    []string
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name:    "param remove single tag",
			noun:    "parameter",
			resName: "/app/param",
			keys:    []string{"env"},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, name string, keys []string) error {
					assert.Equal(t, "/app/param", name)
					assert.Equal(t, []string{"env"}, keys)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Untagged")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name:    "param remove multiple tags",
			noun:    "parameter",
			resName: "/app/param",
			keys:    []string{"env", "team"},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, _ string, keys []string) error {
					assert.Len(t, keys, 2)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 key(s)")
			},
		},
		{
			name:    "param remove tags error",
			noun:    "parameter",
			resName: "/app/param",
			keys:    []string{"env"},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, _ string, _ []string) error {
					return assert.AnError
				},
			},
			wantErr: "failed to remove tags",
		},
		{
			name:    "secret remove single tag",
			noun:    "secret",
			resName: "my-secret",
			keys:    []string{"env"},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, name string, keys []string) error {
					assert.Equal(t, "my-secret", name)
					assert.Equal(t, []string{"env"}, keys)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Untagged")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name:    "secret remove tags error",
			noun:    "secret",
			resName: "my-secret",
			keys:    []string{"env"},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, _ string, _ []string) error {
					return errors.New("AWS error")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			r := &generic.TagRunner{Tagger: tt.store, Noun: tt.noun, Stdout: &buf}
			err := r.RunUntag(t.Context(), tt.resName, tt.keys)

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
