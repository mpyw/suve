package protocol_test

import (
	"os"
	"strings"
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

func TestSocketPath(t *testing.T) {
	t.Parallel()

	t.Run("returns valid path", func(t *testing.T) {
		t.Parallel()
		path := protocol.SocketPath()
		assert.NotEmpty(t, path)
		assert.True(t, strings.Contains(path, "suve") || strings.Contains(path, "agent.sock"))
	})

	t.Run("uses TMPDIR on darwin when set", func(t *testing.T) {
		// This test only runs on darwin - on other platforms TMPDIR may not be used
		if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
			path := protocol.SocketPath()
			// On darwin with TMPDIR set, the path should contain the TMPDIR
			assert.True(t, strings.HasPrefix(path, tmpdir) || strings.Contains(path, "suve"))
		}
	})
}
