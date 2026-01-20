package param_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockLogClient struct {
	getParameterResult *model.Parameter
	getParameterErr    error
	getHistoryResult   *model.ParameterHistory
	getHistoryErr      error
	listParametersErr  error
}

func (m *mockLogClient) GetParameter(_ context.Context, _ string, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

func (m *mockLogClient) GetParameterHistory(_ context.Context, _ string) (*model.ParameterHistory, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}

	if m.getHistoryResult == nil {
		return &model.ParameterHistory{}, nil
	}

	return m.getHistoryResult, nil
}

func (m *mockLogClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	if m.listParametersErr != nil {
		return nil, m.listParametersErr
	}

	return nil, nil
}

func TestLogUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{
					Name: "/app/config", Value: "v1", Version: "1",
					UpdatedAt: timePtr(now.Add(-2 * time.Hour)),
					Metadata:  model.AWSParameterMeta{Type: "String"},
				},
				{
					Name: "/app/config", Value: "v2", Version: "2",
					UpdatedAt: timePtr(now.Add(-1 * time.Hour)),
					Metadata:  model.AWSParameterMeta{Type: "String"},
				},
				{
					Name: "/app/config", Value: "v3", Version: "3",
					UpdatedAt: &now,
					Metadata:  model.AWSParameterMeta{Type: "String"},
				},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.LogInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Len(t, output.Entries, 3)

	// Newest first (default order)
	assert.Equal(t, "3", output.Entries[0].Version)
	assert.Equal(t, "2", output.Entries[1].Version)
	assert.Equal(t, "1", output.Entries[2].Version)

	// IsCurrent flag
	assert.True(t, output.Entries[0].IsCurrent)
	assert.False(t, output.Entries[1].IsCurrent)
	assert.False(t, output.Entries[2].IsCurrent)
}

func TestLogUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name:       "/app/config",
			Parameters: []*model.Parameter{},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.LogInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Empty(t, output.Entries)
}

func TestLogUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		getHistoryErr: errAWS,
	}

	uc := &param.LogUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.LogInput{
		Name: "/app/config",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get parameter history")
}

func TestLogUseCase_Execute_Reverse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-2 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-1 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.LogInput{
		Name:    "/app/config",
		Reverse: true,
	})
	require.NoError(t, err)

	// Oldest first when Reverse is true (keeps AWS order)
	assert.Equal(t, "1", output.Entries[0].Version)
	assert.Equal(t, "2", output.Entries[1].Version)
	assert.Equal(t, "3", output.Entries[2].Version)
}

func TestLogUseCase_Execute_SinceFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-3 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-1 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	since := now.Add(-2 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{
		Name:  "/app/config",
		Since: &since,
	})
	require.NoError(t, err)

	// v1 is before the since filter, so only v2 and v3 should be included
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "3", output.Entries[0].Version)
	assert.Equal(t, "2", output.Entries[1].Version)
}

func TestLogUseCase_Execute_UntilFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-3 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-1 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	until := now.Add(-30 * time.Minute)
	output, err := uc.Execute(t.Context(), param.LogInput{
		Name:  "/app/config",
		Until: &until,
	})
	require.NoError(t, err)

	// v3 is after the until filter, so only v1 and v2 should be included
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "2", output.Entries[0].Version)
	assert.Equal(t, "1", output.Entries[1].Version)
}

func TestLogUseCase_Execute_DateRangeFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-4 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-2 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	since := now.Add(-3 * time.Hour)
	until := now.Add(-1 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{
		Name:  "/app/config",
		Since: &since,
		Until: &until,
	})
	require.NoError(t, err)

	// Only v2 should be within the range
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "2", output.Entries[0].Version)
}

func TestLogUseCase_Execute_NoLastModifiedDate(t *testing.T) {
	t.Parallel()

	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", UpdatedAt: nil, Metadata: model.AWSParameterMeta{Type: "String"}},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.LogInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Nil(t, output.Entries[0].UpdatedAt)
}

func TestLogUseCase_Execute_FilterWithNilLastModifiedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockLogClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", UpdatedAt: nil, Metadata: model.AWSParameterMeta{Type: "String"}},
				{Name: "/app/config", Value: "v2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
			},
		},
	}

	uc := &param.LogUseCase{Client: client}

	since := now.Add(-1 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{
		Name:  "/app/config",
		Since: &since,
	})
	require.NoError(t, err)

	// v1 has nil LastModified, so it is skipped when date filter is applied; only v2 remains
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "2", output.Entries[0].Version)
}

func timePtr(t time.Time) *time.Time {
	return &t
}
