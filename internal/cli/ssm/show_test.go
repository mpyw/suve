package ssm

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

type mockShowClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockShowClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return m.getParameterFunc(ctx, params, optFns...)
}

func (m *mockShowClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	return m.getParameterHistoryFunc(ctx, params, optFns...)
}

func TestShow(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		spec    *version.Spec
		mock    *mockShowClient
		decrypt bool
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: &version.Spec{Name: "/my/param"},
			mock: &mockShowClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
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
			name: "show specific version",
			spec: &version.Spec{Name: "/my/param", Version: aws.Int64(2)},
			mock: &mockShowClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if aws.ToString(params.Name) != "/my/param:2" {
						t.Errorf("expected name with version suffix, got %s", aws.ToString(params.Name))
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             aws.String("/my/param"),
							Value:            aws.String("old-value"),
							Version:          2,
							Type:             types.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("old-value")) {
					t.Errorf("output should contain parameter value")
				}
			},
		},
		{
			name: "show with shift",
			spec: &version.Spec{Name: "/my/param", Shift: 1},
			mock: &mockShowClient{
				getParameterHistoryFunc: func(_ context.Context, params *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
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

			err := Show(context.Background(), tt.mock, &buf, tt.spec, tt.decrypt)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Show() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Show() unexpected error: %v", err)
				return
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

func TestGetParameterWithVersion(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		spec      *version.Spec
		mock      *mockShowClient
		decrypt   bool
		wantValue string
		wantErr   bool
	}{
		{
			name: "direct get without version",
			spec: &version.Spec{Name: "/my/param"},
			mock: &mockShowClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/my/param"),
							Value:   aws.String("current-value"),
							Version: 5,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			wantValue: "current-value",
		},
		{
			name: "with shift ~1 gets previous version",
			spec: &version.Spec{Name: "/my/param", Shift: 1},
			mock: &mockShowClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: aws.Time(now.Add(-2 * time.Hour))},
							{Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
							{Name: aws.String("/my/param"), Value: aws.String("v3"), Version: 3, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			wantValue: "v2",
		},
		{
			name: "shift out of range",
			spec: &version.Spec{Name: "/my/param", Shift: 10},
			mock: &mockShowClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1},
							{Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2},
						},
					}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param, err := GetParameterWithVersion(context.Background(), tt.mock, tt.spec, tt.decrypt)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetParameterWithVersion() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetParameterWithVersion() unexpected error: %v", err)
				return
			}

			if aws.ToString(param.Value) != tt.wantValue {
				t.Errorf("GetParameterWithVersion() Value = %v, want %v", aws.ToString(param.Value), tt.wantValue)
			}
		})
	}
}
