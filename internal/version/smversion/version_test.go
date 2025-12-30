package smversion

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestGetSecretWithVersion_Latest(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			if aws.ToString(params.SecretId) != "my-secret" {
				t.Errorf("expected SecretId my-secret, got %s", aws.ToString(params.SecretId))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:          aws.String("my-secret"),
				ARN:           aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
				VersionId:     aws.String("abc123"),
				SecretString:  aws.String("secret-value"),
				VersionStages: []string{"AWSCURRENT"},
				CreatedDate:   &now,
			}, nil
		},
	}

	spec := &Spec{Name: "my-secret"}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.Name) != "my-secret" {
		t.Errorf("expected name my-secret, got %s", aws.ToString(result.Name))
	}
	if aws.ToString(result.SecretString) != "secret-value" {
		t.Errorf("expected value secret-value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_WithLabel(t *testing.T) {
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			if aws.ToString(params.VersionStage) != "AWSPREVIOUS" {
				t.Errorf("expected VersionStage AWSPREVIOUS, got %s", aws.ToString(params.VersionStage))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:          aws.String("my-secret"),
				VersionId:     aws.String("prev123"),
				SecretString:  aws.String("previous-value"),
				VersionStages: []string{"AWSPREVIOUS"},
			}, nil
		},
	}

	label := "AWSPREVIOUS"
	spec := &Spec{Name: "my-secret", Label: &label}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "previous-value" {
		t.Errorf("expected previous-value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_Shift(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			if aws.ToString(params.SecretId) != "my-secret" {
				t.Errorf("expected SecretId my-secret, got %s", aws.ToString(params.SecretId))
			}
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-2 * time.Hour))},
					{VersionId: aws.String("v2"), CreatedDate: aws.Time(now.Add(-time.Hour))},
					{VersionId: aws.String("v3"), CreatedDate: &now},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			// After sorting by date descending, shift 1 should give v2
			if aws.ToString(params.VersionId) != "v2" {
				t.Errorf("expected VersionId v2, got %s", aws.ToString(params.VersionId))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    aws.String("v2"),
				SecretString: aws.String("v2-value"),
			}, nil
		},
	}

	spec := &Spec{Name: "my-secret", Shift: 1}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "v2-value" {
		t.Errorf("expected v2-value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_ShiftOutOfRange(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: &now},
				},
			}, nil
		},
	}

	spec := &Spec{Name: "my-secret", Shift: 5}
	_, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err == nil {
		t.Fatal("expected error for shift out of range")
	}
	if err.Error() != "version shift out of range: ~5" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetSecretWithVersion_ListVersionsError(t *testing.T) {
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &Spec{Name: "my-secret", Shift: 1}
	_, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "failed to list versions: AWS error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetSecretWithVersion_GetSecretError(t *testing.T) {
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &Spec{Name: "my-secret"}
	_, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "AWS error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetSecretWithVersion_EmptyVersionList(t *testing.T) {
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{},
			}, nil
		},
	}

	spec := &Spec{Name: "my-secret", Shift: 1}
	_, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "secret not found or has no versions: my-secret" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetSecretWithVersion_SortByCreatedDate(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			// Return in random order to verify sorting
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v2"), CreatedDate: aws.Time(now.Add(-time.Hour))},
					{VersionId: aws.String("v3"), CreatedDate: &now},
					{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-2 * time.Hour))},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			// Shift 2 should be the oldest (v1) after sorting by date descending
			if aws.ToString(params.VersionId) != "v1" {
				t.Errorf("expected VersionId v1, got %s", aws.ToString(params.VersionId))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    aws.String("v1"),
				SecretString: aws.String("oldest"),
			}, nil
		},
	}

	spec := &Spec{Name: "my-secret", Shift: 2}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "oldest" {
		t.Errorf("expected oldest, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_NilCreatedDate(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			// Multiple nil dates to ensure both branches of the nil check are covered
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: nil},
					{VersionId: aws.String("v2"), CreatedDate: &now},
					{VersionId: aws.String("v3"), CreatedDate: nil},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			// v2 has a date, v1/v3 don't, so after sorting v2 should be first (index 0)
			// Versions with nil dates are sorted to the end
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    params.VersionId,
				SecretString: aws.String("value"),
			}, nil
		},
	}

	spec := &Spec{Name: "my-secret", Shift: 1}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "value" {
		t.Errorf("expected value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_WithID(t *testing.T) {
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			if aws.ToString(params.VersionId) != "abc123" {
				t.Errorf("expected VersionId abc123, got %s", aws.ToString(params.VersionId))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    aws.String("abc123"),
				SecretString: aws.String("versioned-value"),
			}, nil
		},
	}

	id := "abc123"
	spec := &Spec{Name: "my-secret", ID: &id}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "versioned-value" {
		t.Errorf("expected versioned-value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_IDWithShift(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			// Versions sorted by date descending after sort: v3(newest), v2, v1(oldest)
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-2 * time.Hour))},
					{VersionId: aws.String("v2"), CreatedDate: aws.Time(now.Add(-time.Hour))},
					{VersionId: aws.String("v3"), CreatedDate: &now},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			// ID=v2 is at index 1 after sorting, shift 1 should give v1 (index 2)
			if aws.ToString(params.VersionId) != "v1" {
				t.Errorf("expected VersionId v1, got %s", aws.ToString(params.VersionId))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    aws.String("v1"),
				SecretString: aws.String("v1-value"),
			}, nil
		},
	}

	id := "v2"
	spec := &Spec{Name: "my-secret", ID: &id, Shift: 1}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "v1-value" {
		t.Errorf("expected v1-value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_LabelWithShift(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			// v3 has AWSCURRENT, v2 has AWSPREVIOUS
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-2 * time.Hour)), VersionStages: []string{}},
					{VersionId: aws.String("v2"), CreatedDate: aws.Time(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
					{VersionId: aws.String("v3"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			// AWSCURRENT=v3 is at index 0 after sorting, shift 1 should give v2 (index 1)
			if aws.ToString(params.VersionId) != "v2" {
				t.Errorf("expected VersionId v2, got %s", aws.ToString(params.VersionId))
			}
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    aws.String("v2"),
				SecretString: aws.String("v2-value"),
			}, nil
		},
	}

	label := "AWSCURRENT"
	spec := &Spec{Name: "my-secret", Label: &label, Shift: 1}
	result, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aws.ToString(result.SecretString) != "v2-value" {
		t.Errorf("expected v2-value, got %s", aws.ToString(result.SecretString))
	}
}

func TestGetSecretWithVersion_IDNotFound(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: &now},
				},
			}, nil
		},
	}

	id := "nonexistent"
	spec := &Spec{Name: "my-secret", ID: &id, Shift: 1}
	_, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err == nil {
		t.Fatal("expected error for ID not found")
	}
	if err.Error() != "version ID not found: nonexistent" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetSecretWithVersion_LabelNotFound(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("v1"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
				},
			}, nil
		},
	}

	label := "NONEXISTENT"
	spec := &Spec{Name: "my-secret", Label: &label, Shift: 1}
	_, err := GetSecretWithVersion(t.Context(), mock, spec)

	if err == nil {
		t.Fatal("expected error for label not found")
	}
	if err.Error() != "version label not found: NONEXISTENT" {
		t.Errorf("unexpected error message: %v", err)
	}
}
