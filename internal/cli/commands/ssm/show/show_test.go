package show_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/ssm/show"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "show"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve ssm show")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "show", "/app/param#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})
}

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return m.getParameterFunc(ctx, params, optFns...)
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	return m.getParameterHistoryFunc(ctx, params, optFns...)
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
				Spec: &ssmversion.Spec{Name: "/my/param"},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr("test-value"),
							Version:          3,
							Type:             types.ParameterTypeString,
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
				Spec: &ssmversion.Spec{Name: "/my/param", Shift: 1},
			},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
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
				Spec:       &ssmversion.Spec{Name: "/my/param"},
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             lo.ToPtr("/my/param"),
							Value:            lo.ToPtr(`{"zebra":"last","apple":"first"}`),
							Version:          1,
							Type:             types.ParameterTypeString,
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
			name: "error from AWS",
			opts: show.Options{Spec: &ssmversion.Spec{Name: "/my/param"}},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "show without LastModifiedDate",
			opts: show.Options{Spec: &ssmversion.Spec{Name: "/my/param"}},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("test-value"),
							Version: 1,
							Type:    types.ParameterTypeString,
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
				Spec:       &ssmversion.Spec{Name: "/my/param"},
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("a,b,c"),
							Version: 1,
							Type:    types.ParameterTypeStringList,
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
				Spec:       &ssmversion.Spec{Name: "/my/param"},
				Decrypt:    false,
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("encrypted-blob"),
							Version: 1,
							Type:    types.ParameterTypeSecureString,
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
				Spec:       &ssmversion.Spec{Name: "/my/param"},
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/my/param"),
							Value:   lo.ToPtr("not json"),
							Version: 1,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "not json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &show.Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
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
