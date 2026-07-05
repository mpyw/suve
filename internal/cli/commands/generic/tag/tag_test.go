package tag_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// TestCommand_Validation exercises the wired param and secret tag/untag commands
// end-to-end through the app, covering argument validation for both providers.
func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"param tag missing arguments", []string{"suve", "param", "tag"}, "usage:"},
		{"param tag missing tag argument", []string{"suve", "param", "tag", "/app/param"}, "usage:"},
		{"param tag invalid tag format", []string{"suve", "param", "tag", "/app/param", "invalid"}, "expected key=value"},
		{"param tag empty key", []string{"suve", "param", "tag", "/app/param", "=value"}, "key cannot be empty"},
		{"param untag missing arguments", []string{"suve", "param", "untag"}, "usage:"},
		{"param untag missing key argument", []string{"suve", "param", "untag", "/app/param"}, "usage:"},
		{"secret tag missing arguments", []string{"suve", "secret", "tag"}, "usage:"},
		{"secret tag missing tag argument", []string{"suve", "secret", "tag", "my-secret"}, "usage:"},
		{"secret tag invalid tag format", []string{"suve", "secret", "tag", "my-secret", "invalid"}, "expected key=value"},
		{"secret tag empty key", []string{"suve", "secret", "tag", "my-secret", "=value"}, "key cannot be empty"},
		{"secret untag missing arguments", []string{"suve", "secret", "untag"}, "usage:"},
		{"secret untag missing key argument", []string{"suve", "secret", "untag", "my-secret"}, "usage:"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := apptest.AWSApp()
			err := app.Run(t.Context(), tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

func TestRunTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		noun    string
		resName string
		tags    map[string]string
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name:    "param add single tag",
			noun:    "parameter",
			resName: "/app/param",
			tags:    map[string]string{"env": "prod"},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, name string, add map[string]string) error {
					assert.Equal(t, "/app/param", name)
					assert.Len(t, add, 1)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tagged")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name:    "param add multiple tags",
			noun:    "parameter",
			resName: "/app/param",
			tags:    map[string]string{"env": "prod", "team": "backend"},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, _ string, add map[string]string) error {
					assert.Len(t, add, 2)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 tag(s)")
			},
		},
		{
			name:    "param add tags error",
			noun:    "parameter",
			resName: "/app/param",
			tags:    map[string]string{"env": "prod"},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
					return assert.AnError
				},
			},
			wantErr: "failed to add tags",
		},
		{
			name:    "secret add single tag",
			noun:    "secret",
			resName: "my-secret",
			tags:    map[string]string{"env": "prod"},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, name string, add map[string]string) error {
					assert.Equal(t, "my-secret", name)
					assert.Len(t, add, 1)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tagged")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name:    "secret add multiple tags",
			noun:    "secret",
			resName: "my-secret",
			tags:    map[string]string{"env": "prod", "team": "backend"},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, _ string, add map[string]string) error {
					assert.Len(t, add, 2)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 tag(s)")
			},
		},
		{
			name:    "secret tag resource error",
			noun:    "secret",
			resName: "my-secret",
			tags:    map[string]string{"env": "prod"},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
					return errors.New("AWS error")
				},
			},
			wantErr: "failed to add tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			r := &generictag.Runner{Tagger: tt.store, Noun: tt.noun, Stdout: &buf}
			err := r.RunTag(t.Context(), tt.resName, tt.tags)

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

			r := &generictag.Runner{Tagger: tt.store, Noun: tt.noun, Stdout: &buf}
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
