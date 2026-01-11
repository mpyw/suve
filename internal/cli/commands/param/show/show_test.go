package show_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/show"
	"github.com/mpyw/suve/internal/cli/output"
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
	getParameterFunc        func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error)
	listTagsForResourceFunc func(ctx context.Context, params *paramapi.ListTagsForResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.ListTagsForResourceOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	return m.getParameterFunc(ctx, params, optFns...)
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	return m.getParameterHistoryFunc(ctx, params, optFns...)
}

func (m *mockClient) ListTagsForResource(ctx context.Context, params *paramapi.ListTagsForResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.ListTagsForResourceOutput, error) {
	if m.listTagsForResourceFunc != nil {
		return m.listTagsForResourceFunc(ctx, params, optFns...)
	}
	return &paramapi.ListTagsForResourceOutput{}, nil
}

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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr("test-value"),
							Version:          3,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: &now},
							{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr(`{"zebra":"last","apple":"first"}`),
							Version:          1,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				appleIdx := bytes.Index([]byte(output), []byte("apple"))
				zebraIdx := bytes.Index([]byte(output), []byte("zebra"))
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "show without LastModifiedDate",
			opts: show.Options{Spec: &paramversion.Spec{Name: "/my/param"}},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("test-value"),
							Version: 1,
							Type:    paramapi.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("a,b,c"),
							Version: 1,
							Type:    paramapi.ParameterTypeStringList,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("encrypted-blob"),
							Version: 1,
							Type:    paramapi.ParameterTypeSecureString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("not json"),
							Version: 1,
							Type:    paramapi.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr("raw-value"),
							Version:          1,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr(`{"zebra":"last","apple":"first"}`),
							Version:          1,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				appleIdx := bytes.Index([]byte(output), []byte("apple"))
				zebraIdx := bytes.Index([]byte(output), []byte("zebra"))
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr("test-value"),
							Version:          1,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
				listTagsForResourceFunc: func(_ context.Context, _ *paramapi.ListTagsForResourceInput, _ ...func(*paramapi.Options)) (*paramapi.ListTagsForResourceOutput, error) {
					return &paramapi.ListTagsForResourceOutput{
						TagList: []paramapi.Tag{
							{Key: lo.ToPtr("Environment"), Value: lo.ToPtr("production")},
							{Key: lo.ToPtr("Team"), Value: lo.ToPtr("backend")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr("test-value"),
							Version:          1,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
				listTagsForResourceFunc: func(_ context.Context, _ *paramapi.ListTagsForResourceInput, _ ...func(*paramapi.Options)) (*paramapi.ListTagsForResourceOutput, error) {
					return &paramapi.ListTagsForResourceOutput{
						TagList: []paramapi.Tag{
							{Key: lo.ToPtr("Environment"), Value: lo.ToPtr("production")},
							{Key: lo.ToPtr("Team"), Value: lo.ToPtr("backend")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("test-value"),
							Version: 1,
							Type:    paramapi.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
