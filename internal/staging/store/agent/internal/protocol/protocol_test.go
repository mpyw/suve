package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

func TestResponse_Err(t *testing.T) {
	t.Parallel()

	t.Run("success - returns nil", func(t *testing.T) {
		t.Parallel()
		resp := &protocol.Response{Success: true}
		assert.NoError(t, resp.Err())
	})

	t.Run("error - returns ErrNotStaged", func(t *testing.T) {
		t.Parallel()
		resp := &protocol.Response{
			Success: false,
			Error:   staging.ErrNotStaged.Error(),
		}
		err := resp.Err()
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("error - returns generic error", func(t *testing.T) {
		t.Parallel()
		resp := &protocol.Response{
			Success: false,
			Error:   "some other error",
		}
		err := resp.Err()
		assert.Error(t, err)
		assert.Equal(t, "some other error", err.Error())
	})
}
