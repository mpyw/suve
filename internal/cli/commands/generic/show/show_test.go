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
	genericshow "github.com/mpyw/suve/internal/cli/commands/generic/show"
	cmdparam "github.com/mpyw/suve/internal/cli/commands/param"
	cmdsecret "github.com/mpyw/suve/internal/cli/commands/secret"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/version/paramversion"
	"github.com/mpyw/suve/internal/version/secretversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"param missing name", []string{"suve", "param", "show"}, "usage: suve param show"},
		{"param invalid version spec", []string{"suve", "param", "show", "/app/param#"}, "must be followed by"},
		{"secret missing name", []string{"suve", "secret", "show"}, "usage: suve secret show"},
		{"secret invalid version spec", []string{"suve", "secret", "show", "my-secret#"}, "must be followed by"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
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

func run(
	t *testing.T, presenter genericshow.Presenter, opts genericshow.Options,
) (string, error) {
	t.Helper()

	var buf, errBuf bytes.Buffer

	r := &genericshow.Runner{Presenter: presenter, Options: opts, Stdout: &buf, Stderr: &errBuf}
	err := r.Run(t.Context())

	return buf.String(), err
}

func mustParseParam(t *testing.T, s string) *paramversion.Spec {
	t.Helper()

	spec, err := paramversion.Parse(s)
	require.NoError(t, err)

	return spec
}

//nolint:funlen // Table-driven test with many cases
func TestRunParam(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		spec    string
		opts    genericshow.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: "/my/param",
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
			spec: "/my/param~1",
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
			spec: "/my/param",
			opts: genericshow.Options{ParseJSON: true},
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
			spec: "/my/param",
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
			spec: "/my/param",
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
			spec: "/my/param",
			opts: genericshow.Options{ParseJSON: true},
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
			spec: "/my/param",
			opts: genericshow.Options{ParseJSON: true},
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
			spec: "/my/param",
			opts: genericshow.Options{ParseJSON: true},
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
			spec: "/my/param",
			opts: genericshow.Options{Raw: true},
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
			spec: "/my/param~1",
			opts: genericshow.Options{Raw: true},
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
			spec: "/my/param",
			opts: genericshow.Options{ParseJSON: true, Raw: true},
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
			spec: "/my/param",
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
			spec: "/my/param",
			opts: genericshow.Options{Output: output.FormatJSON},
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
			spec: "/my/param",
			opts: genericshow.Options{Output: output.FormatJSON},
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

			presenter := cmdparam.NewShowPresenter(tt.store, mustParseParam(t, tt.spec))
			out, err := run(t, presenter, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

const testARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"

//nolint:funlen // Table-driven test with many cases
func TestRunSecret(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		spec    *secretversion.Spec
		opts    genericshow.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: &secretversion.Spec{Name: "my-secret"},
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
			spec: &secretversion.Spec{Name: "my-secret", Shift: 1},
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
			spec: &secretversion.Spec{Name: "my-secret"},
			opts: genericshow.Options{ParseJSON: true},
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
			spec: &secretversion.Spec{Name: "my-secret"},
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
			spec: &secretversion.Spec{Name: "my-secret"},
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
			spec: &secretversion.Spec{Name: "my-secret"},
			opts: genericshow.Options{ParseJSON: true},
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
			spec: &secretversion.Spec{Name: "my-secret"},
			opts: genericshow.Options{Raw: true},
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
			spec: &secretversion.Spec{Name: "my-secret", Shift: 1},
			opts: genericshow.Options{Raw: true},
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
			spec: &secretversion.Spec{Name: "my-secret"},
			opts: genericshow.Options{ParseJSON: true, Raw: true},
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
			spec: &secretversion.Spec{Name: "my-secret"},
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
			spec: &secretversion.Spec{Name: "my-secret"},
			opts: genericshow.Options{Output: output.FormatJSON},
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
			spec: &secretversion.Spec{Name: "my-secret"},
			opts: genericshow.Options{Output: output.FormatJSON},
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

			presenter := cmdsecret.NewShowPresenter(tt.store, tt.spec)
			out, err := run(t, presenter, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
