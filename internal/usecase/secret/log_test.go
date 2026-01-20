package secret_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockLogClient struct {
	versionsResult []*model.SecretVersion
	versionsErr    error
	getSecretValue map[string]string
	getSecretErr   map[string]error
}

func (m *mockLogClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	if m.versionsErr != nil {
		return nil, m.versionsErr
	}

	return m.versionsResult, nil
}

func (m *mockLogClient) GetSecret(_ context.Context, _ string, versionID, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		if err, ok := m.getSecretErr[versionID]; ok {
			return nil, err
		}
	}

	if m.getSecretValue != nil {
		if value, ok := m.getSecretValue[versionID]; ok {
			return &model.Secret{Version: versionID, Value: value}, nil
		}
	}

	return nil, errors.New("not found")
}

func (m *mockLogClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	return nil, errors.New("not implemented")
}

func TestLogUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-2 * time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{}}},
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{}}},
			{Version: "v3-id", CreatedAt: ptrTime(now), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
		},
		getSecretValue: map[string]string{
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
		versionsResult: []*model.SecretVersion{},
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
		versionsErr: errors.New("aws error"),
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
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-2 * time.Hour))},
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
			{Version: "v3-id", CreatedAt: ptrTime(now)},
		},
		getSecretValue: map[string]string{
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
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-3 * time.Hour))},
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
			{Version: "v3-id", CreatedAt: ptrTime(now)},
		},
		getSecretValue: map[string]string{
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
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-3 * time.Hour))},
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
			{Version: "v3-id", CreatedAt: ptrTime(now)},
		},
		getSecretValue: map[string]string{
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
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now)},
		},
		getSecretErr: map[string]error{
			"v1-id": errors.New("get value error"),
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	// GetSecret errors are swallowed; value will be empty
	assert.Len(t, output.Entries, 1)
	assert.Empty(t, output.Entries[0].Value)
}

func TestLogUseCase_Execute_NilCreatedDate(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: nil},
		},
		getSecretValue: map[string]string{
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
		versionsResult: []*model.SecretVersion{
			{Version: "", CreatedAt: ptrTime(now)},
		},
	}

	uc := &secret.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Empty(t, output.Entries[0].VersionID)
	assert.Empty(t, output.Entries[0].Value) // Value fetch is skipped when Version is empty
}

func TestLogUseCase_Execute_SortWithNilCreatedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	// Test sorting when some entries have nil CreatedAt
	client := &mockLogClient{
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: nil},
			{Version: "v2-id", CreatedAt: ptrTime(now)},
			{Version: "v3-id", CreatedAt: nil},
		},
		getSecretValue: map[string]string{
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
	// Test sorting when entries have the same CreatedAt
	client := &mockLogClient{
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now)},
			{Version: "v2-id", CreatedAt: ptrTime(now)},
		},
		getSecretValue: map[string]string{
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
		versionsResult: []*model.SecretVersion{
			// Oldest first in input
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-2 * time.Hour))},
			// Newest in input
			{Version: "v3-id", CreatedAt: ptrTime(now)},
			// Middle in input
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
		},
		getSecretValue: map[string]string{
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
	// Test date filtering when entry has nil CreatedAt (should be filtered out when date filter is applied)
	client := &mockLogClient{
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: nil},
			{Version: "v2-id", CreatedAt: ptrTime(now)},
		},
		getSecretValue: map[string]string{
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
	// v1 has nil CreatedAt, so it is skipped when date filter is applied; only v2 remains
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "v2-id", output.Entries[0].VersionID)
}
