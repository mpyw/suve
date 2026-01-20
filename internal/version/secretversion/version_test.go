package secretversion_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/version/secretversion"
)

type mockClient struct {
	getSecretFunc         func(ctx context.Context, name, versionID, versionStage string) (*model.Secret, error)
	getSecretVersionsFunc func(ctx context.Context, name string) ([]*model.SecretVersion, error)
}

func (m *mockClient) GetSecret(ctx context.Context, name, versionID, versionStage string) (*model.Secret, error) {
	if m.getSecretFunc != nil {
		return m.getSecretFunc(ctx, name, versionID, versionStage)
	}

	return nil, fmt.Errorf("GetSecret not mocked")
}

func (m *mockClient) GetSecretVersions(ctx context.Context, name string) ([]*model.SecretVersion, error) {
	if m.getSecretVersionsFunc != nil {
		return m.getSecretVersionsFunc(ctx, name)
	}

	return nil, fmt.Errorf("GetSecretVersions not mocked")
}

func (m *mockClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	return nil, fmt.Errorf("ListSecrets not mocked")
}

func TestGetSecretWithVersion_Latest(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretFunc: func(_ context.Context, name, versionID, versionStage string) (*model.Secret, error) {
			assert.Equal(t, "my-secret", name)
			assert.Empty(t, versionID)
			assert.Empty(t, versionStage)

			return &model.Secret{
				Name:      "my-secret",
				Value:     "secret-value",
				Version:   "abc123",
				CreatedAt: &now,
				Metadata: model.AWSSecretMeta{
					ARN:           "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
					VersionStages: []string{"AWSCURRENT"},
				},
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret"}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "my-secret", result.Name)
	assert.Equal(t, "secret-value", result.Value)
}

func TestGetSecretWithVersion_WithLabel(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		getSecretFunc: func(_ context.Context, name, versionID, versionStage string) (*model.Secret, error) {
			assert.Equal(t, "AWSPREVIOUS", versionStage)

			return &model.Secret{
				Name:    "my-secret",
				Version: "prev123",
				Value:   "previous-value",
				Metadata: model.AWSSecretMeta{
					VersionStages: []string{"AWSPREVIOUS"},
				},
			}, nil
		},
	}

	label := "AWSPREVIOUS"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: &label}}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "previous-value", result.Value)
}

func TestGetSecretWithVersion_Shift(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretVersionsFunc: func(_ context.Context, name string) ([]*model.SecretVersion, error) {
			assert.Equal(t, "my-secret", name)

			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: lo.ToPtr(now.Add(-2 * time.Hour))},
				{Version: "v2", CreatedAt: lo.ToPtr(now.Add(-time.Hour))},
				{Version: "v3", CreatedAt: &now},
			}, nil
		},
		getSecretFunc: func(_ context.Context, name, versionID, versionStage string) (*model.Secret, error) {
			// After sorting by date descending, shift 1 should give v2
			assert.Equal(t, "v2", versionID)

			return &model.Secret{
				Name:    "my-secret",
				Version: "v2",
				Value:   "v2-value",
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "v2-value", result.Value)
}

func TestGetSecretWithVersion_ShiftOutOfRange(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: &now},
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
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
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
		getSecretFunc: func(_ context.Context, _, _, _ string) (*model.Secret, error) {
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
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			return []*model.SecretVersion{}, nil
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
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			// Return in random order to verify sorting
			return []*model.SecretVersion{
				{Version: "v2", CreatedAt: lo.ToPtr(now.Add(-time.Hour))},
				{Version: "v3", CreatedAt: &now},
				{Version: "v1", CreatedAt: lo.ToPtr(now.Add(-2 * time.Hour))},
			}, nil
		},
		getSecretFunc: func(_ context.Context, _, versionID, _ string) (*model.Secret, error) {
			// Shift 2 should be the oldest (v1) after sorting by date descending
			assert.Equal(t, "v1", versionID)

			return &model.Secret{
				Name:    "my-secret",
				Version: "v1",
				Value:   "oldest",
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 2}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "oldest", result.Value)
}

func TestGetSecretWithVersion_NilCreatedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			// Multiple nil dates to ensure both branches of the nil check are covered
			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: nil},
				{Version: "v2", CreatedAt: &now},
				{Version: "v3", CreatedAt: nil},
			}, nil
		},
		getSecretFunc: func(_ context.Context, _, versionID, _ string) (*model.Secret, error) {
			// v2 has a date, v1/v3 don't, so after sorting v2 should be first (index 0)
			// Versions with nil dates are sorted to the end
			return &model.Secret{
				Name:    "my-secret",
				Version: versionID,
				Value:   "value",
			}, nil
		},
	}

	spec := &secretversion.Spec{Name: "my-secret", Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "value", result.Value)
}

func TestGetSecretWithVersion_WithID(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		getSecretFunc: func(_ context.Context, _, versionID, _ string) (*model.Secret, error) {
			assert.Equal(t, "abc123", versionID)

			return &model.Secret{
				Name:    "my-secret",
				Version: "abc123",
				Value:   "versioned-value",
			}, nil
		},
	}

	id := "abc123"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{ID: &id}}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "versioned-value", result.Value)
}

func TestGetSecretWithVersion_IDWithShift(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			// Versions sorted by date descending after sort: v3(newest), v2, v1(oldest)
			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: lo.ToPtr(now.Add(-2 * time.Hour))},
				{Version: "v2", CreatedAt: lo.ToPtr(now.Add(-time.Hour))},
				{Version: "v3", CreatedAt: &now},
			}, nil
		},
		getSecretFunc: func(_ context.Context, _, versionID, _ string) (*model.Secret, error) {
			// ID=v2 is at index 1 after sorting, shift 1 should give v1 (index 2)
			assert.Equal(t, "v1", versionID)

			return &model.Secret{
				Name:    "my-secret",
				Version: "v1",
				Value:   "v1-value",
			}, nil
		},
	}

	id := "v2"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{ID: &id}, Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "v1-value", result.Value)
}

func TestGetSecretWithVersion_LabelWithShift(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			// v3 has AWSCURRENT, v2 has AWSPREVIOUS
			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: lo.ToPtr(now.Add(-2 * time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{}}},
				{Version: "v2", CreatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
				{Version: "v3", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
			}, nil
		},
		getSecretFunc: func(_ context.Context, _, versionID, _ string) (*model.Secret, error) {
			// AWSCURRENT=v3 is at index 0 after sorting, shift 1 should give v2 (index 1)
			assert.Equal(t, "v2", versionID)

			return &model.Secret{
				Name:    "my-secret",
				Version: "v2",
				Value:   "v2-value",
			}, nil
		},
	}

	label := "AWSCURRENT"
	spec := &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: &label}, Shift: 1}
	result, err := secretversion.GetSecretWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "v2-value", result.Value)
}

func TestGetSecretWithVersion_IDNotFound(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: &now},
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
		getSecretVersionsFunc: func(_ context.Context, _ string) ([]*model.SecretVersion, error) {
			return []*model.SecretVersion{
				{Version: "v1", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
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
