package ssmversion

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

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

func TestGetParameterWithVersion_Latest(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			if aws.ToString(params.Name) != "/my/param" {
				t.Errorf("expected name /my/param, got %s", aws.ToString(params.Name))
			}
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
	}

	spec := &Spec{Name: "/my/param"}
	result, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.Name) != "/my/param" {
		t.Errorf("expected name /my/param, got %s", aws.ToString(result.Name))
	}
	if aws.ToString(result.Value) != "test-value" {
		t.Errorf("expected value test-value, got %s", aws.ToString(result.Value))
	}
	if result.Version != 3 {
		t.Errorf("expected version 3, got %d", result.Version)
	}
}

func TestGetParameterWithVersion_SpecificVersion(t *testing.T) {
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			if aws.ToString(params.Name) != "/my/param:2" {
				t.Errorf("expected name /my/param:2, got %s", aws.ToString(params.Name))
			}
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    aws.String("/my/param"),
					Value:   aws.String("old-value"),
					Version: 2,
					Type:    types.ParameterTypeString,
				},
			}, nil
		},
	}

	v := int64(2)
	spec := &Spec{Name: "/my/param", Absolute: AbsoluteSpec{Version: &v}}
	result, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.Value) != "old-value" {
		t.Errorf("expected value old-value, got %s", aws.ToString(result.Value))
	}
	if result.Version != 2 {
		t.Errorf("expected version 2, got %d", result.Version)
	}
}

func TestGetParameterWithVersion_Shift(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, params *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			if aws.ToString(params.Name) != "/my/param" {
				t.Errorf("expected name /my/param, got %s", aws.ToString(params.Name))
			}
			// History is returned oldest first by AWS
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: aws.Time(now.Add(-2 * time.Hour))},
					{Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
					{Name: aws.String("/my/param"), Value: aws.String("v3"), Version: 3, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	spec := &Spec{Name: "/my/param", Shift: 1}
	result, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Shift 1 means one version back from latest (v3), so v2
	if aws.ToString(result.Value) != "v2" {
		t.Errorf("expected value v2, got %s", aws.ToString(result.Value))
	}
	if result.Version != 2 {
		t.Errorf("expected version 2, got %d", result.Version)
	}
}

func TestGetParameterWithVersion_ShiftFromSpecificVersion(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: aws.Time(now.Add(-2 * time.Hour))},
					{Name: aws.String("/my/param"), Value: aws.String("v2"), Version: 2, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
					{Name: aws.String("/my/param"), Value: aws.String("v3"), Version: 3, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	v := int64(3)
	spec := &Spec{Name: "/my/param", Absolute: AbsoluteSpec{Version: &v}, Shift: 2}
	result, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Version 3, shift 2 means v3 -> v2 -> v1
	if aws.ToString(result.Value) != "v1" {
		t.Errorf("expected value v1, got %s", aws.ToString(result.Value))
	}
}

func TestGetParameterWithVersion_ShiftOutOfRange(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	spec := &Spec{Name: "/my/param", Shift: 5}
	_, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err == nil {
		t.Fatal("expected error for shift out of range")
	}
	if err.Error() != "version shift out of range: ~5" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetParameterWithVersion_VersionNotFound(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: aws.String("/my/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	v := int64(99)
	spec := &Spec{Name: "/my/param", Absolute: AbsoluteSpec{Version: &v}, Shift: 1}
	_, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err == nil {
		t.Fatal("expected error for version not found")
	}
	if err.Error() != "version 99 not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetParameterWithVersion_EmptyHistory(t *testing.T) {
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{},
			}, nil
		},
	}

	spec := &Spec{Name: "/my/param", Shift: 1}
	_, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err == nil {
		t.Fatal("expected error for empty history")
	}
	if err.Error() != "parameter not found: /my/param" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetParameterWithVersion_GetParameterError(t *testing.T) {
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &Spec{Name: "/my/param"}
	_, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "failed to get parameter: AWS error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetParameterWithVersion_GetParameterHistoryError(t *testing.T) {
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &Spec{Name: "/my/param", Shift: 1}
	_, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "failed to get parameter history: AWS error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetParameterWithVersion_DecryptFlag(t *testing.T) {
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			if !aws.ToBool(params.WithDecryption) {
				t.Errorf("expected WithDecryption to be true")
			}
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    aws.String("/my/param"),
					Value:   aws.String("decrypted-value"),
					Version: 1,
					Type:    types.ParameterTypeSecureString,
				},
			}, nil
		},
	}

	spec := &Spec{Name: "/my/param"}
	result, err := GetParameterWithVersion(t.Context(), mock, spec, true)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.Value) != "decrypted-value" {
		t.Errorf("expected decrypted-value, got %s", aws.ToString(result.Value))
	}
}
