package param_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockLogClient struct {
	getHistoryResult *paramapi.GetParameterHistoryOutput
	getHistoryErr    error
}

func (m *mockLogClient) GetParameterHistory(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}
	return m.getHistoryResult, nil
}

func TestLogUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, Type: paramapi.ParameterTypeString, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, Type: paramapi.ParameterTypeString, LastModifiedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3, Type: paramapi.ParameterTypeString, LastModifiedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.LogInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Len(t, output.Entries, 3)

	// Newest first (default order)
	assert.Equal(t, int64(3), output.Entries[0].Version)
	assert.Equal(t, int64(2), output.Entries[1].Version)
	assert.Equal(t, int64(1), output.Entries[2].Version)

	// IsCurrent flag
	assert.True(t, output.Entries[0].IsCurrent)
	assert.False(t, output.Entries[1].IsCurrent)
	assert.False(t, output.Entries[2].IsCurrent)
}

func TestLogUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.LogInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Empty(t, output.Entries)
}

func TestLogUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		getHistoryErr: errors.New("aws error"),
	}

	uc := &param.LogUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.LogInput{
		Name: "/app/config",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get parameter history")
}

func TestLogUseCase_Execute_Reverse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.LogInput{
		Name:    "/app/config",
		Reverse: true,
	})
	require.NoError(t, err)

	// Oldest first when Reverse is true (keeps AWS order)
	assert.Equal(t, int64(1), output.Entries[0].Version)
	assert.Equal(t, int64(2), output.Entries[1].Version)
	assert.Equal(t, int64(3), output.Entries[2].Version)
}

func TestLogUseCase_Execute_SinceFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-3 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	since := now.Add(-2 * time.Hour)
	output, err := uc.Execute(context.Background(), param.LogInput{
		Name:  "/app/config",
		Since: &since,
	})
	require.NoError(t, err)

	// v1 is before the since filter, so only v2 and v3 should be included
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, int64(3), output.Entries[0].Version)
	assert.Equal(t, int64(2), output.Entries[1].Version)
}

func TestLogUseCase_Execute_UntilFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-3 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	until := now.Add(-30 * time.Minute)
	output, err := uc.Execute(context.Background(), param.LogInput{
		Name:  "/app/config",
		Until: &until,
	})
	require.NoError(t, err)

	// v3 is after the until filter, so only v1 and v2 should be included
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, int64(2), output.Entries[0].Version)
	assert.Equal(t, int64(1), output.Entries[1].Version)
}

func TestLogUseCase_Execute_DateRangeFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-4 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	since := now.Add(-3 * time.Hour)
	until := now.Add(-1 * time.Hour)
	output, err := uc.Execute(context.Background(), param.LogInput{
		Name:  "/app/config",
		Since: &since,
		Until: &until,
	})
	require.NoError(t, err)

	// Only v2 should be within the range
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, int64(2), output.Entries[0].Version)
}

func TestLogUseCase_Execute_NoLastModifiedDate(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: nil},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.LogInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Nil(t, output.Entries[0].LastModified)
}

func TestLogUseCase_Execute_FilterWithNilLastModifiedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: nil},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now)},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	since := now.Add(-1 * time.Hour)
	output, err := uc.Execute(context.Background(), param.LogInput{
		Name:  "/app/config",
		Since: &since,
	})
	require.NoError(t, err)

	// v1 has nil LastModifiedDate, so it is skipped when date filter is applied; only v2 remains
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, int64(2), output.Entries[0].Version)
}
