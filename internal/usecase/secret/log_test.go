package secret_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockLogClient struct {
	listVersionsResult  *secretapi.ListSecretVersionIDsOutput
	listVersionsErr     error
	getSecretValueValue map[string]string
	getSecretValueErr   map[string]error
}

func (m *mockLogClient) ListSecretVersionIds(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
	if m.listVersionsErr != nil {
		return nil, m.listVersionsErr
	}

	return m.listVersionsResult, nil
}

func (m *mockLogClient) GetSecretValue(_ context.Context, input *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	versionID := lo.FromPtr(input.VersionId)
	if m.getSecretValueErr != nil {
		if err, ok := m.getSecretValueErr[versionID]; ok {
			return nil, err
		}
	}

	if m.getSecretValueValue != nil {
		if value, ok := m.getSecretValueValue[versionID]; ok {
			return &secretapi.GetSecretValueOutput{SecretString: lo.ToPtr(value)}, nil
		}
	}

	return nil, &secretapi.ResourceNotFoundException{Message: lo.ToPtr("not found")}
}

func TestLogUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour)), VersionStages: []string{}},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour)), VersionStages: []string{}},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now), VersionStages: []string{"AWSCURRENT"}},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
			"v3-id": "value-3",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Len(t, output.Entries, 3)

	// Default order is newest first
	assert.Equal(t, "v3-id", output.Entries[0].VersionID)
	assert.True(t, output.Entries[0].IsCurrent)
	assert.Equal(t, "v2-id", output.Entries[1].VersionID)
	assert.Equal(t, "v1-id", output.Entries[2].VersionID)
}

func TestLogUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{},
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestLogUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		listVersionsErr: errors.New("aws error"),
	}

	uc := &secret.LogUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secret versions")
}

func TestLogUseCase_Execute_Reverse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
			"v3-id": "value-3",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name:    "my-secret",
		Reverse: true,
	})
	require.NoError(t, err)

	// Oldest first when Reverse is true
	assert.Equal(t, "v1-id", output.Entries[0].VersionID)
	assert.Equal(t, "v2-id", output.Entries[1].VersionID)
	assert.Equal(t, "v3-id", output.Entries[2].VersionID)
}

func TestLogUseCase_Execute_SinceFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-3 * time.Hour))},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueValue: map[string]string{
			"v2-id": "value-2",
			"v3-id": "value-3",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	since := now.Add(-2 * time.Hour)
	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name:  "my-secret",
		Since: &since,
	})
	require.NoError(t, err)

	// v1 is before the since filter
	assert.Len(t, output.Entries, 2)
}

func TestLogUseCase_Execute_UntilFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-3 * time.Hour))},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	until := now.Add(-30 * time.Minute)
	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name:  "my-secret",
		Until: &until,
	})
	require.NoError(t, err)

	// v3 is after the until filter
	assert.Len(t, output.Entries, 2)
}

func TestLogUseCase_Execute_GetValueError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueErr: map[string]error{
			"v1-id": errors.New("get value error"),
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	// GetSecretValue errors are swallowed; value will be empty
	assert.Len(t, output.Entries, 1)
	assert.Empty(t, output.Entries[0].Value)
}

func TestLogUseCase_Execute_NilCreatedDate(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: nil},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Nil(t, output.Entries[0].CreatedDate)
}

func TestLogUseCase_Execute_NilVersionId(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: nil, CreatedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Empty(t, output.Entries[0].VersionID)
	assert.Empty(t, output.Entries[0].Value) // Value fetch is skipped when VersionId is nil
}

func TestLogUseCase_Execute_SortWithNilCreatedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Test sorting when some entries have nil CreatedDate
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: nil},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now)},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: nil},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
			"v3-id": "value-3",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 3)
}

func TestLogUseCase_Execute_SortWithEqualDates(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Test sorting when entries have the same CreatedDate
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now)},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestLogUseCase_Execute_SortUnsortedInput(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Test sorting when input is in unsorted order (oldest first)
	// This ensures the sorting "a.Before(b)" branch is covered
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				// Oldest first in input
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				// Newest in input
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now)},
				// Middle in input
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
			"v3-id": "value-3",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 3)

	// After sorting, should be newest first
	assert.Equal(t, "v3-id", output.Entries[0].VersionID)
	assert.Equal(t, "v2-id", output.Entries[1].VersionID)
	assert.Equal(t, "v1-id", output.Entries[2].VersionID)
}

func TestLogUseCase_Execute_FilterWithNilCreatedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Test date filtering when entry has nil CreatedDate (should be filtered out when date filter is applied)
	client := &mockLogClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: nil},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueValue: map[string]string{
			"v1-id": "value-1",
			"v2-id": "value-2",
		},
	}

	uc := &secret.LogUseCase{Client: client}

	since := now.Add(-1 * time.Hour)
	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name:  "my-secret",
		Since: &since,
	})
	require.NoError(t, err)
	// v1 has nil CreatedDate, so it is skipped when date filter is applied; only v2 remains
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "v2-id", output.Entries[0].VersionID)
}
