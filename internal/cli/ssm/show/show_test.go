package show

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/mpyw/suve/internal/version"
)

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
	now := time.Now()

	tests := []struct {
		name    string
		spec    *version.Spec
		mock    *mockClient
		decrypt bool
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: &version.Spec{Name: "/my/param"},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             aws.String("/my/param"),
							Value:            aws.String("test-value"),
							Version:          3,
							Type:             types.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("/my/param")) {
					t.Errorf("output should contain parameter name")
				}
				if !bytes.Contains([]byte(output), []byte("test-value")) {
					t.Errorf("output should contain parameter value")
				}
			},
		},
		{
			name: "show with shift",
			spec: &version.Spec{Name: "/my/param", Shift: 1},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/my/param"), Value: aws.String("v3"), Version: 3, LastModifiedDate: &now},
							{Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
							{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: aws.Time(now.Add(-2 * time.Hour))},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("v2")) {
					t.Errorf("output should contain shifted version value")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := Run(t.Context(), tt.mock, &buf, tt.spec, tt.decrypt)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Run() unexpected error: %v", err)
				return
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
