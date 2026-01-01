package cat_test

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

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/ssm/cat"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "cat"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve ssm cat")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "cat", "/app/param#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})
}

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		opts    cat.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "output raw value",
			opts: cat.Options{
				Spec:    &ssmversion.Spec{Name: "/app/param"},
				Decrypt: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             lo.ToPtr("/app/param"),
							Value:            lo.ToPtr("raw-value"),
							Version:          1,
							Type:             types.ParameterTypeString,
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
			name: "output with shift",
			opts: cat.Options{
				Spec:    &ssmversion.Spec{Name: "/app/param", Shift: 1},
				Decrypt: true,
			},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Equal(t, "v1", output)
			},
		},
		{
			name: "output JSON formatted with sorted keys",
			opts: cat.Options{
				Spec:       &ssmversion.Spec{Name: "/app/param"},
				Decrypt:    true,
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             lo.ToPtr("/app/param"),
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
			opts: cat.Options{
				Spec:    &ssmversion.Spec{Name: "/app/param"},
				Decrypt: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "json flag with StringList warns",
			opts: cat.Options{
				Spec:       &ssmversion.Spec{Name: "/app/param"},
				Decrypt:    true,
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("a,b,c"),
							Version: 1,
							Type:    types.ParameterTypeStringList,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Equal(t, "a,b,c", output)
			},
		},
		{
			name: "json flag with encrypted SecureString warns",
			opts: cat.Options{
				Spec:       &ssmversion.Spec{Name: "/app/param"},
				Decrypt:    false,
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("encrypted-blob"),
							Version: 1,
							Type:    types.ParameterTypeSecureString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Equal(t, "encrypted-blob", output)
			},
		},
		{
			name: "json flag with non-JSON value warns",
			opts: cat.Options{
				Spec:       &ssmversion.Spec{Name: "/app/param"},
				Decrypt:    true,
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("not json"),
							Version: 1,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Equal(t, "not json", output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, warnBuf bytes.Buffer
			r := &cat.Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &warnBuf,
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
