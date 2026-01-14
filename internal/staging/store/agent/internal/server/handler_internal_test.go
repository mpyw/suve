package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuccessResponse(t *testing.T) {
	t.Parallel()

	resp := successResponse()
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)
	assert.Nil(t, resp.Data)
}

func TestErrorResponse(t *testing.T) {
	t.Parallel()

	err := errors.New("test error")
	resp := errorResponse(err)
	assert.False(t, resp.Success)
	assert.Equal(t, "test error", resp.Error)
	assert.Nil(t, resp.Data)
}

func TestErrorMessageResponse(t *testing.T) {
	t.Parallel()

	resp := errorMessageResponse("custom message")
	assert.False(t, resp.Success)
	assert.Equal(t, "custom message", resp.Error)
	assert.Nil(t, resp.Data)
}

func TestMarshalResponse(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		data := map[string]string{"key": "value"}
		resp := marshalResponse(data)
		assert.True(t, resp.Success)
		assert.Empty(t, resp.Error)
		assert.JSONEq(t, `{"key":"value"}`, string(resp.Data))
	})

	t.Run("error on unmarshalable type", func(t *testing.T) {
		t.Parallel()

		// Channels cannot be marshaled to JSON
		ch := make(chan int)
		resp := marshalResponse(ch)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "json")
	})
}
