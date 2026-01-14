package update

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/api/paramapi"
)

type mockGetParameterClient struct {
	output *paramapi.GetParameterOutput
	err    error
}

//nolint:lll // mock function signature
func (m *mockGetParameterClient) GetParameter(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.output, nil
}

func TestGetCurrentValue(t *testing.T) {
	t.Parallel()

	t.Run("returns value when parameter exists", func(t *testing.T) {
		t.Parallel()

		client := &mockGetParameterClient{
			output: &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:  lo.ToPtr("/app/config"),
					Value: lo.ToPtr("test-value"),
				},
			},
		}

		value, ok := getCurrentValue(context.Background(), client, "/app/config")
		assert.True(t, ok)
		assert.Equal(t, "test-value", value)
	})

	t.Run("returns false when error occurs", func(t *testing.T) {
		t.Parallel()

		client := &mockGetParameterClient{
			err: &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		}

		value, ok := getCurrentValue(context.Background(), client, "/app/missing")
		assert.False(t, ok)
		assert.Empty(t, value)
	})

	t.Run("returns false when parameter is nil", func(t *testing.T) {
		t.Parallel()

		client := &mockGetParameterClient{
			output: &paramapi.GetParameterOutput{
				Parameter: nil,
			},
		}

		value, ok := getCurrentValue(context.Background(), client, "/app/config")
		assert.False(t, ok)
		assert.Empty(t, value)
	})

	t.Run("returns false when value is nil", func(t *testing.T) {
		t.Parallel()

		client := &mockGetParameterClient{
			output: &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:  lo.ToPtr("/app/config"),
					Value: nil,
				},
			},
		}

		value, ok := getCurrentValue(context.Background(), client, "/app/config")
		assert.False(t, ok)
		assert.Empty(t, value)
	})
}
