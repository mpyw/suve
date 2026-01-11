package param_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

type mockDiffClient struct {
	getParameterResults []*paramapi.GetParameterOutput
	getParameterErrs    []error
	getParameterCalls   int
	// historyParams stores the base data; each call returns a fresh copy
	historyParams []paramapi.ParameterHistory
	getHistoryErr error
}

func (m *mockDiffClient) GetParameter(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	idx := m.getParameterCalls
	m.getParameterCalls++

	if idx < len(m.getParameterErrs) && m.getParameterErrs[idx] != nil {
		return nil, m.getParameterErrs[idx]
	}
	if idx < len(m.getParameterResults) {
		return m.getParameterResults[idx], nil
	}
	return nil, errors.New("unexpected GetParameter call")
}

func (m *mockDiffClient) GetParameterHistory(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}
	// Return a fresh copy to avoid in-place mutations affecting subsequent calls
	params := make([]paramapi.ParameterHistory, len(m.historyParams))
	copy(params, m.historyParams)
	return &paramapi.GetParameterHistoryOutput{Parameters: params}, nil
}

func TestDiffUseCase_Execute(t *testing.T) {
	t.Parallel()

	// #VERSION specs without shift use GetParameter (with name:version format)
	client := &mockDiffClient{
		getParameterResults: []*paramapi.GetParameterOutput{
			{Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("old-value"), Version: 1}},
			{Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("new-value"), Version: 2}},
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#1")
	spec2, _ := paramversion.Parse("/app/config#2")

	output, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.OldName)
	assert.Equal(t, int64(1), output.OldVersion)
	assert.Equal(t, "old-value", output.OldValue)
	assert.Equal(t, "/app/config", output.NewName)
	assert.Equal(t, int64(2), output.NewVersion)
	assert.Equal(t, "new-value", output.NewValue)
}

func TestDiffUseCase_Execute_Spec1Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getParameterErrs: []error{errors.New("get parameter error")},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#1")
	spec2, _ := paramversion.Parse("/app/config#2")

	_, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_Spec2Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getParameterResults: []*paramapi.GetParameterOutput{
			{Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("old-value"), Version: 1}},
		},
		getParameterErrs: []error{nil, errors.New("second get parameter error")},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#1")
	spec2, _ := paramversion.Parse("/app/config#2")

	_, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_WithLatest(t *testing.T) {
	t.Parallel()

	// Both specs without shift use GetParameter
	client := &mockDiffClient{
		getParameterResults: []*paramapi.GetParameterOutput{
			{Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("old-value"), Version: 3}},
			{Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("latest-value"), Version: 5}},
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#3")
	spec2, _ := paramversion.Parse("/app/config")

	output, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), output.OldVersion)
	assert.Equal(t, int64(5), output.NewVersion)
}

func TestDiffUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	// Specs with shift use GetParameterHistory
	client := &mockDiffClient{
		historyParams: []paramapi.ParameterHistory{
			{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1},
			{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2},
			{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3},
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config~2") // 2 versions back from latest (v3 -> v1)
	spec2, _ := paramversion.Parse("/app/config~1") // 1 version back from latest (v3 -> v2)

	output, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), output.OldVersion)
	assert.Equal(t, "v1", output.OldValue)
	assert.Equal(t, int64(2), output.NewVersion)
	assert.Equal(t, "v2", output.NewValue)
}

func TestDiffUseCase_Execute_WithShift_Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getHistoryErr: errors.New("history error"),
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config~1")
	spec2, _ := paramversion.Parse("/app/config")

	_, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}
