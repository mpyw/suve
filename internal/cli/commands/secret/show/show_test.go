package show_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/show"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "show"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve secret show")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "show", "my-secret#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})
}

// showStore builds a mock reader that resolves to the latest ref and returns the
// given entry.
func showStore(entry *domain.Entry) *providermock.Store {
	return &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return entry, nil
		},
	}
}

const testARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"

//nolint:funlen // Table-driven test with many cases
func TestRun(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		opts    show.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			store: showStore(&domain.Entry{
				Name:    "my-secret",
				Value:   "secret-value",
				Version: domain.Version{ID: "abc123", Label: "AWSCURRENT", Created: &now},
				Extra:   []domain.Field{{Label: "ARN", Value: testARN}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "my-secret")
				assert.Contains(t, output, "secret-value")
			},
		},
		{
			name: "show with shift",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret", Shift: 1}},
			store: showStore(&domain.Entry{
				Name:    "my-secret",
				Value:   "previous-value",
				Version: domain.Version{ID: "v2"},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "previous-value")
			},
		},
		{
			name: "show JSON formatted with sorted keys",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true},
			store: showStore(&domain.Entry{
				Name:    "my-secret",
				Value:   `{"zebra":"last","apple":"first"}`,
				Version: domain.Version{ID: "abc123", Label: "AWSCURRENT", Created: &now},
				Extra:   []domain.Field{{Label: "ARN", Value: testARN}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()

				appleIdx := strings.Index(output, "apple")
				zebraIdx := strings.Index(output, "zebra")

				require.NotEqual(t, -1, appleIdx, "expected apple in output")
				require.NotEqual(t, -1, zebraIdx, "expected zebra in output")
				assert.Less(t, appleIdx, zebraIdx, "expected keys to be sorted (apple before zebra)")
			},
		},
		{
			name: "error from AWS",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			store: &providermock.Store{
				ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
					return provider.VersionRef{}, nil
				},
				GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
					return nil, assert.AnError
				},
			},
			wantErr: true,
		},
		{
			name: "show without optional fields",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			store: showStore(&domain.Entry{
				Name:  "my-secret",
				Value: "secret-value",
				Extra: []domain.Field{{Label: "ARN", Value: testARN}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "my-secret")
				assert.NotContains(t, output, "VersionId")
				assert.NotContains(t, output, "Stages")
				assert.NotContains(t, output, "Created")
			},
		},
		{
			name: "json flag with non-JSON value warns",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true},
			store: showStore(&domain.Entry{
				Name:  "my-secret",
				Value: "not json",
				Extra: []domain.Field{{Label: "ARN", Value: testARN}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "not json")
			},
		},
		{
			name: "raw mode outputs only value",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Raw: true},
			store: showStore(&domain.Entry{
				Name:  "my-secret",
				Value: "raw-secret-value",
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Equal(t, "raw-secret-value", output)
			},
		},
		{
			name: "raw mode with shift",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret", Shift: 1}, Raw: true},
			store: showStore(&domain.Entry{
				Name:  "my-secret",
				Value: "previous-value",
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Equal(t, "previous-value", output)
			},
		},
		{
			name: "raw mode with JSON formatting",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true, Raw: true},
			store: showStore(&domain.Entry{
				Name:  "my-secret",
				Value: `{"zebra":"last","apple":"first"}`,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()

				appleIdx := strings.Index(output, "apple")
				zebraIdx := strings.Index(output, "zebra")

				require.NotEqual(t, -1, appleIdx, "expected apple in output")
				require.NotEqual(t, -1, zebraIdx, "expected zebra in output")
				assert.Less(t, appleIdx, zebraIdx, "expected keys to be sorted (apple before zebra)")
			},
		},
		{
			name: "show with tags",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			store: showStore(&domain.Entry{
				Name:    "my-secret",
				Value:   "secret-value",
				Version: domain.Version{ID: "abc123", Label: "AWSCURRENT", Created: &now},
				Extra:   []domain.Field{{Label: "ARN", Value: testARN}},
				Tags: []domain.Tag{
					{Key: "Environment", Value: "production"},
					{Key: "Team", Value: "backend"},
				},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tags")
				assert.Contains(t, output, "2 tag(s)")
				assert.Contains(t, output, "Environment")
				assert.Contains(t, output, "production")
				assert.Contains(t, output, "Team")
				assert.Contains(t, output, "backend")
			},
		},
		{
			name: "show with tags in JSON output",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Output: output.FormatJSON},
			store: showStore(&domain.Entry{
				Name:    "my-secret",
				Value:   "secret-value",
				Version: domain.Version{ID: "abc123", Label: "AWSCURRENT", Created: &now},
				Extra:   []domain.Field{{Label: "ARN", Value: testARN}},
				Tags: []domain.Tag{
					{Key: "Environment", Value: "production"},
					{Key: "Team", Value: "backend"},
				},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"tags"`)
				assert.Contains(t, output, `"Environment"`)
				assert.Contains(t, output, `"production"`)
				assert.Contains(t, output, `"Team"`)
				assert.Contains(t, output, `"backend"`)
			},
		},
		{
			name: "JSON output with empty tags shows empty object",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Output: output.FormatJSON},
			store: showStore(&domain.Entry{
				Name:  "my-secret",
				Value: "secret-value",
				Extra: []domain.Field{{Label: "ARN", Value: testARN}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"tags": {}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &show.Runner{
				UseCase: &secret.ShowUseCase{Reader: tt.store},
				Stdout:  &buf,
				Stderr:  &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

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
