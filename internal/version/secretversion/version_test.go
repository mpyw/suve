package secretversion_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/version/secretversion"
)

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretapi.ListSecretVersionIDsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretapi.ListSecretVersionIDsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestGetSecretWithVersion_Latest(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			return &secretapi.GetSecretValueOutput{
				Name:          lo.ToPtr("my-secret"),
				ARN:           lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
				VersionId:     lo.ToPtr("abc123"),
				SecretString:  lo.ToPtr("secret-value"),
				VersionStages: []string{"AWSCURRENT"},
				CreatedDate:   &now,
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret"}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "my-secret", lo.FromPtr(result.Name))
	assert.Equal(t, "secret-value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_WithLabel(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			assert.Equal(t, "AWSPREVIOUS", lo.FromPtr(params.VersionStage))
			return &secretapi.GetSecretValueOutput{
				Name:          lo.ToPtr("my-secret"),
				VersionId:     lo.ToPtr("prev123"),
				SecretString:  lo.ToPtr("previous-value"),
				VersionStages: []string{"AWSPREVIOUS"},
			}, nil
		},
	}

	label := "AWSPREVIOUS"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: &label}}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "previous-value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_Shift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, params *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
					{VersionId: lo.ToPtr("v2"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
					{VersionId: lo.ToPtr("v3"), CreatedDate: &now},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			// After sorting by date descending, shift 1 should give v2
			assert.Equal(t, "v2", lo.FromPtr(params.VersionId))
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    lo.ToPtr("v2"),
				SecretString: lo.ToPtr("v2-value"),
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "v2-value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_ShiftOutOfRange(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: &now},
				},
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 5}
	_, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "version shift out of range: ~5", err.Error())
}

func TestGetSecretWithVersion_ListVersionsError(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 1}
	_, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "failed to list versions: AWS error", err.Error())
}

func TestGetSecretWithVersion_GetSecretError(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &secretversion.Spec{Name: "my-secret"}
	_, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "AWS error", err.Error())
}

func TestGetSecretWithVersion_EmptyVersionList(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{},
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 1}
	_, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "secret not found or has no versions: my-secret", err.Error())
}

func TestGetSecretWithVersion_SortByCreatedDate(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			// Return in random order to verify sorting
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v2"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
					{VersionId: lo.ToPtr("v3"), CreatedDate: &now},
					{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			// Shift 2 should be the oldest (v1) after sorting by date descending
			assert.Equal(t, "v1", lo.FromPtr(params.VersionId))
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    lo.ToPtr("v1"),
				SecretString: lo.ToPtr("oldest"),
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 2}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "oldest", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_NilCreatedDate(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			// Multiple nil dates to ensure both branches of the nil check are covered
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: nil},
					{VersionId: lo.ToPtr("v2"), CreatedDate: &now},
					{VersionId: lo.ToPtr("v3"), CreatedDate: nil},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			// v2 has a date, v1/v3 don't, so after sorting v2 should be first (index 0)
			// Versions with nil dates are sorted to the end
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    params.VersionId,
				SecretString: lo.ToPtr("value"),
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_WithID(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			assert.Equal(t, "abc123", lo.FromPtr(params.VersionId))
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    lo.ToPtr("abc123"),
				SecretString: lo.ToPtr("versioned-value"),
			}, nil
		},
	}

	id := "abc123"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{ID: &id}}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "versioned-value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_IDWithShift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			// Versions sorted by date descending after sort: v3(newest), v2, v1(oldest)
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
					{VersionId: lo.ToPtr("v2"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
					{VersionId: lo.ToPtr("v3"), CreatedDate: &now},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			// ID=v2 is at index 1 after sorting, shift 1 should give v1 (index 2)
			assert.Equal(t, "v1", lo.FromPtr(params.VersionId))
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    lo.ToPtr("v1"),
				SecretString: lo.ToPtr("v1-value"),
			}, nil
		},
	}

	id := "v2"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{ID: &id}, Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "v1-value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_LabelWithShift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			// v3 has AWSCURRENT, v2 has AWSPREVIOUS
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour)), VersionStages: []string{}},
					{VersionId: lo.ToPtr("v2"), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
					{VersionId: lo.ToPtr("v3"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
				},
			}, nil
		},
		getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			// AWSCURRENT=v3 is at index 0 after sorting, shift 1 should give v2 (index 1)
			assert.Equal(t, "v2", lo.FromPtr(params.VersionId))
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    lo.ToPtr("v2"),
				SecretString: lo.ToPtr("v2-value"),
			}, nil
		},
	}

	label := "AWSCURRENT"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: &label}, Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "v2-value", lo.FromPtr(result.SecretString))
}

func TestGetSecretWithVersion_IDNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: &now},
				},
			}, nil
		},
	}

	id := "nonexistent"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{ID: &id}, Shift: 1}
	_, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "version ID not found: nonexistent", err.Error())
}

func TestGetSecretWithVersion_LabelNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
			return &secretapi.ListSecretVersionIDsOutput{
				Versions: []secretapi.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("v1"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
				},
			}, nil
		},
	}

	label := "NONEXISTENT"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: &label}, Shift: 1}
	_, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "version label not found: NONEXISTENT", err.Error())
}

func TestTruncateVersionID(t *testing.T) {
	t.Parallel()

	t.Run("long ID - truncate to 8", func(t *testing.T) {
		t.Parallel()
		result := secretversion.TruncateVersionID("abcdefgh-1234-5678-9abc-def012345678")
		assert.Equal(t, "abcdefgh", result)
	})

	t.Run("exactly 8 chars", func(t *testing.T) {
		t.Parallel()
		result := secretversion.TruncateVersionID("12345678")
		assert.Equal(t, "12345678", result)
	})

	t.Run("short ID - no truncation", func(t *testing.T) {
		t.Parallel()
		result := secretversion.TruncateVersionID("abc")
		assert.Equal(t, "abc", result)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		result := secretversion.TruncateVersionID("")
		assert.Empty(t, result)
	})
}
