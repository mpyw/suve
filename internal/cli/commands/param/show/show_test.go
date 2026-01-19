package show_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/show"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/model"
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

type mockClient struct {
	getParameterFunc        func(ctx context.Context, name string, version string) (*model.Parameter, error)
	getParameterHistoryFunc func(ctx context.Context, name string) (*model.ParameterHistory, error)
	getTagsFunc             func(ctx context.Context, name string) (map[string]string, error)
}

func (m *mockClient) GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error) {
	return m.getParameterFunc(ctx, name, version)
}

func (m *mockClient) GetParameterHistory(ctx context.Context, name string) (*model.ParameterHistory, error) {
	return m.getParameterHistoryFunc(ctx, name)
}

func (m *mockClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	return nil, nil
}

func (m *mockClient) GetTags(ctx context.Context, name string) (map[string]string, error) {
	if m.getTagsFunc != nil {
		return m.getTagsFunc(ctx, name)
	}

	return nil, nil //nolint:nilnil // return empty tags by default
}

func (m *mockClient) AddTags(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (m *mockClient) RemoveTags(_ context.Context, _ string, _ []string) error {
	return nil
}

//nolint:funlen // Table-driven test with many cases
func TestRun(t *testing.T) {
	t.Parallel()

	now := time.Now()
	t1 := now.Add(-1 * time.Hour)
	t2 := now.Add(-2 * time.Hour)

	tests := []struct {
		name    string
		opts    show.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			opts: show.Options{
				Spec: &paramversion.Spec{Name: "/my/param"},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:         "/my/param",
						Value:        "test-value",
						Version:      "3",
						Type:         "String",
						LastModified: &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/my/param")
				assert.Contains(t, output, "test-value")
			},
		},
		{
			name: "show with shift",
			opts: show.Options{
				Spec: &paramversion.Spec{Name: "/my/param", Shift: 1},
			},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
					return &model.ParameterHistory{
						Name: "/my/param",
						Parameters: []*model.Parameter{
							{Name: "/my/param", Value: "v3", Version: "3", LastModified: &now},
							{Name: "/my/param", Value: "v2", Version: "2", LastModified: &t1},
							{Name: "/my/param", Value: "v1", Version: "1", LastModified: &t2},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "v2")
			},
		},
		{
			name: "show JSON formatted",
			opts: show.Options{
				Spec:      &paramversion.Spec{Name: "/my/param"},
				ParseJSON: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:         "/my/param",
						Value:        `{"zebra":"last","apple":"first"}`,
						Version:      "1",
						Type:         "String",
						LastModified: &now,
					}, nil
				},
			},
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
			opts: show.Options{Spec: &paramversion.Spec{Name: "/my/param"}},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "show without LastModifiedDate",
			opts: show.Options{Spec: &paramversion.Spec{Name: "/my/param"}},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:    "/my/param",
						Value:   "test-value",
						Version: "1",
						Type:    "String",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/my/param")
				assert.NotContains(t, output, "Modified")
			},
		},
		{
			name: "json flag with StringList warns",
			opts: show.Options{
				Spec:      &paramversion.Spec{Name: "/my/param"},
				ParseJSON: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:    "/my/param",
						Value:   "a,b,c",
						Version: "1",
						Type:    "StringList",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "a,b,c")
			},
		},
		{
			name: "json flag with encrypted SecureString warns",
			opts: show.Options{
				Spec:      &paramversion.Spec{Name: "/my/param"},
				ParseJSON: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:    "/my/param",
						Value:   "encrypted-blob",
						Version: "1",
						Type:    "SecureString",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

				assert.Contains(t, output, "encrypted-blob")
			},
		},
		{
			name: "json flag with non-JSON value warns",
			opts: show.Options{
				Spec:      &paramversion.Spec{Name: "/my/param"},
				ParseJSON: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:    "/my/param",
						Value:   "not json",
						Version: "1",
						Type:    "String",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

				assert.Contains(t, output, "not json")
				assert.NotContains(t, output, "JsonParsed")
			},
		},
		{
			name: "raw mode outputs only value",
			opts: show.Options{
				Spec: &paramversion.Spec{Name: "/my/param"},
				Raw:  true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:         "/my/param",
						Value:        "raw-value",
						Version:      "1",
						Type:         "String",
						LastModified: &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

				assert.Equal(t, "raw-value", output)
			},
		},
		{
			name: "raw mode with shift",
			opts: show.Options{
				Spec: &paramversion.Spec{Name: "/my/param", Shift: 1},
				Raw:  true,
			},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
					return &model.ParameterHistory{
						Name: "/my/param",
						Parameters: []*model.Parameter{
							{Name: "/my/param", Value: "v1", Version: "1", LastModified: &t1},
							{Name: "/my/param", Value: "v2", Version: "2", LastModified: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

				assert.Equal(t, "v1", output)
			},
		},
		{
			name: "raw mode with JSON formatting",
			opts: show.Options{
				Spec:      &paramversion.Spec{Name: "/my/param"},
				ParseJSON: true,
				Raw:       true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:         "/my/param",
						Value:        `{"zebra":"last","apple":"first"}`,
						Version:      "1",
						Type:         "String",
						LastModified: &now,
					}, nil
				},
			},
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
			opts: show.Options{
				Spec: &paramversion.Spec{Name: "/my/param"},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:         "/my/param",
						Value:        "test-value",
						Version:      "1",
						Type:         "String",
						LastModified: &now,
					}, nil
				},
				getTagsFunc: func(_ context.Context, _ string) (map[string]string, error) {
					return map[string]string{
						"Environment": "production",
						"Team":        "backend",
					}, nil
				},
			},
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
			opts: show.Options{
				Spec:   &paramversion.Spec{Name: "/my/param"},
				Output: output.FormatJSON,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:         "/my/param",
						Value:        "test-value",
						Version:      "1",
						Type:         "String",
						LastModified: &now,
					}, nil
				},
				getTagsFunc: func(_ context.Context, _ string) (map[string]string, error) {
					return map[string]string{
						"Environment": "production",
						"Team":        "backend",
					}, nil
				},
			},
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
			opts: show.Options{
				Spec:   &paramversion.Spec{Name: "/my/param"},
				Output: output.FormatJSON,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return &model.Parameter{
						Name:    "/my/param",
						Value:   "test-value",
						Version: "1",
						Type:    "String",
					}, nil
				},
			},
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
				UseCase: &param.ShowUseCase{Client: tt.mock},
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
