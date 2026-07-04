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
	"github.com/mpyw/suve/internal/cli/commands/param/show"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "show"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve param show")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "show", "/app/param#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})
}

// showStore builds a mock that resolves to latest and returns the given entry.
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

func mustParse(t *testing.T, s string) *paramversion.Spec {
	t.Helper()

	spec, err := paramversion.Parse(s)
	require.NoError(t, err)

	return spec
}

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
			opts: show.Options{Spec: mustParse(t, "/my/param")},
			store: showStore(&domain.Entry{
				Name:     "/my/param",
				Value:    "test-value",
				Version:  domain.Version{ID: "3"},
				Type:     domain.ValueTypePlaintext,
				Modified: &now,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/my/param")
				assert.Contains(t, output, "test-value")
			},
		},
		{
			name: "show with shift",
			opts: show.Options{Spec: mustParse(t, "/my/param~1")},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "v2",
				Version: domain.Version{ID: "2"},
				Type:    domain.ValueTypePlaintext,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "v2")
			},
		},
		{
			name: "show JSON formatted",
			opts: show.Options{Spec: mustParse(t, "/my/param"), ParseJSON: true},
			store: showStore(&domain.Entry{
				Name:     "/my/param",
				Value:    `{"zebra":"last","apple":"first"}`,
				Version:  domain.Version{ID: "1"},
				Type:     domain.ValueTypePlaintext,
				Modified: &now,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()

				appleIdx := strings.Index(output, "apple")
				zebraIdx := strings.Index(output, "zebra")

				require.NotEqual(t, -1, appleIdx, "expected apple in output")
				require.NotEqual(t, -1, zebraIdx, "expected zebra in output")
				assert.Less(t, appleIdx, zebraIdx, "expected keys to be sorted (apple before zebra)")
				assert.Contains(t, output, "JsonParsed")
			},
		},
		{
			name: "error from AWS",
			opts: show.Options{Spec: mustParse(t, "/my/param")},
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
			name: "show without LastModifiedDate",
			opts: show.Options{Spec: mustParse(t, "/my/param")},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "test-value",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypePlaintext,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/my/param")
				assert.NotContains(t, output, "Modified")
			},
		},
		{
			name: "json flag with StringList warns",
			opts: show.Options{Spec: mustParse(t, "/my/param"), ParseJSON: true},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "a,b,c",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypeList,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "a,b,c")
			},
		},
		{
			name: "json flag with encrypted SecureString warns",
			opts: show.Options{Spec: mustParse(t, "/my/param"), ParseJSON: true},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "encrypted-blob",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypeSecret,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "encrypted-blob")
			},
		},
		{
			name: "json flag with non-JSON value warns",
			opts: show.Options{Spec: mustParse(t, "/my/param"), ParseJSON: true},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "not json",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypePlaintext,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "not json")
				assert.NotContains(t, output, "JsonParsed")
			},
		},
		{
			name: "raw mode outputs only value",
			opts: show.Options{Spec: mustParse(t, "/my/param"), Raw: true},
			store: showStore(&domain.Entry{
				Name:     "/my/param",
				Value:    "raw-value",
				Version:  domain.Version{ID: "1"},
				Type:     domain.ValueTypePlaintext,
				Modified: &now,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Equal(t, "raw-value", output)
			},
		},
		{
			name: "raw mode with shift",
			opts: show.Options{Spec: mustParse(t, "/my/param~1"), Raw: true},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "v1",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypePlaintext,
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Equal(t, "v1", output)
			},
		},
		{
			name: "raw mode with JSON formatting",
			opts: show.Options{Spec: mustParse(t, "/my/param"), ParseJSON: true, Raw: true},
			store: showStore(&domain.Entry{
				Name:     "/my/param",
				Value:    `{"zebra":"last","apple":"first"}`,
				Version:  domain.Version{ID: "1"},
				Type:     domain.ValueTypePlaintext,
				Modified: &now,
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
			opts: show.Options{Spec: mustParse(t, "/my/param")},
			store: showStore(&domain.Entry{
				Name:     "/my/param",
				Value:    "test-value",
				Version:  domain.Version{ID: "1"},
				Type:     domain.ValueTypePlaintext,
				Modified: &now,
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
			opts: show.Options{Spec: mustParse(t, "/my/param"), Output: output.FormatJSON},
			store: showStore(&domain.Entry{
				Name:     "/my/param",
				Value:    "test-value",
				Version:  domain.Version{ID: "1"},
				Type:     domain.ValueTypePlaintext,
				Modified: &now,
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
			opts: show.Options{Spec: mustParse(t, "/my/param"), Output: output.FormatJSON},
			store: showStore(&domain.Entry{
				Name:    "/my/param",
				Value:   "test-value",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypePlaintext,
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
				UseCase: &param.ShowUseCase{Reader: tt.store},
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
