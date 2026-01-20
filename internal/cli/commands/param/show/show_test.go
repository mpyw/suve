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
	getParameterResult   *model.Parameter
	getParameterErr      error
	getHistoryResult     *model.ParameterHistory
	getHistoryErr        error
	listParametersResult []*model.ParameterListItem
	listParametersErr    error
	getTagsResult        map[string]string
	getTagsErr           error
}

func (m *mockClient) GetParameter(_ context.Context, _ string, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

func (m *mockClient) GetParameterHistory(_ context.Context, _ string) (*model.ParameterHistory, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}

	if m.getHistoryResult == nil {
		return &model.ParameterHistory{}, nil
	}

	return m.getHistoryResult, nil
}

func (m *mockClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	if m.listParametersErr != nil {
		return nil, m.listParametersErr
	}

	return m.listParametersResult, nil
}

func (m *mockClient) GetTags(_ context.Context, _ string) (map[string]string, error) {
	if m.getTagsErr != nil {
		return nil, m.getTagsErr
	}

	return m.getTagsResult, nil
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
				getParameterResult: &model.Parameter{
					Name:      "/my/param",
					Value:     "test-value",
					Version:   "3",
					UpdatedAt: &now,
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/my/param",
					Parameters: []*model.Parameter{
						{Name: "/my/param", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/my/param", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{
							Name: "/my/param", Value: "v1", Version: "1",
							UpdatedAt: timePtr(now.Add(-2 * time.Hour)),
							Metadata:  model.AWSParameterMeta{Type: "String"},
						},
					},
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
				getParameterResult: &model.Parameter{
					Name:      "/my/param",
					Value:     `{"zebra":"last","apple":"first"}`,
					Version:   "1",
					UpdatedAt: &now,
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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
			name:    "error from AWS",
			opts:    show.Options{Spec: &paramversion.Spec{Name: "/my/param"}},
			mock:    &mockClient{getParameterErr: fmt.Errorf("AWS error")},
			wantErr: true,
		},
		{
			name: "show without LastModifiedDate",
			opts: show.Options{Spec: &paramversion.Spec{Name: "/my/param"}},
			mock: &mockClient{
				getParameterResult: &model.Parameter{
					Name:    "/my/param",
					Value:   "test-value",
					Version: "1",
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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
				getParameterResult: &model.Parameter{
					Name:    "/my/param",
					Value:   "a,b,c",
					Version: "1",
					Metadata: model.AWSParameterMeta{
						Type: "StringList",
					},
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
				getParameterResult: &model.Parameter{
					Name:    "/my/param",
					Value:   "encrypted-blob",
					Version: "1",
					Metadata: model.AWSParameterMeta{
						Type: "SecureString",
					},
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
				getParameterResult: &model.Parameter{
					Name:    "/my/param",
					Value:   "not json",
					Version: "1",
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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
				getParameterResult: &model.Parameter{
					Name:      "/my/param",
					Value:     "raw-value",
					Version:   "1",
					UpdatedAt: &now,
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/my/param",
					Parameters: []*model.Parameter{
						{Name: "/my/param", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/my/param", Value: "v2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
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
				getParameterResult: &model.Parameter{
					Name:      "/my/param",
					Value:     `{"zebra":"last","apple":"first"}`,
					Version:   "1",
					UpdatedAt: &now,
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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
				getParameterResult: &model.Parameter{
					Name:      "/my/param",
					Value:     "test-value",
					Version:   "1",
					UpdatedAt: &now,
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
				},
				getTagsResult: map[string]string{
					"Environment": "production",
					"Team":        "backend",
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
				getParameterResult: &model.Parameter{
					Name:      "/my/param",
					Value:     "test-value",
					Version:   "1",
					UpdatedAt: &now,
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
				},
				getTagsResult: map[string]string{
					"Environment": "production",
					"Team":        "backend",
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
				getParameterResult: &model.Parameter{
					Name:    "/my/param",
					Value:   "test-value",
					Version: "1",
					Metadata: model.AWSParameterMeta{
						Type: "String",
					},
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

func timePtr(t time.Time) *time.Time {
	return &t
}
