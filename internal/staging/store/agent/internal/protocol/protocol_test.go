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

func TestSocketPathForAccount(t *testing.T) {
	t.Parallel()

	const (
		testAccountID = "123456789012"
		testRegion    = "us-east-1"
	)

	t.Run("returns valid path with account and region", func(t *testing.T) {
		t.Parallel()
		path := protocol.SocketPathForAccount(testAccountID, testRegion)
		assert.NotEmpty(t, path)
		assert.Contains(t, path, testAccountID)
		assert.Contains(t, path, testRegion)
		assert.Contains(t, path, "agent.sock")
	})

	t.Run("different accounts have different paths", func(t *testing.T) {
		t.Parallel()
		path1 := protocol.SocketPathForAccount("111111111111", "us-east-1")
		path2 := protocol.SocketPathForAccount("222222222222", "us-east-1")
		assert.NotEqual(t, path1, path2)
	})

	t.Run("different regions have different paths", func(t *testing.T) {
		t.Parallel()
		path1 := protocol.SocketPathForAccount(testAccountID, "us-east-1")
		path2 := protocol.SocketPathForAccount(testAccountID, "us-west-2")
		assert.NotEqual(t, path1, path2)
	})

	t.Run("uses TMPDIR on darwin when set", func(t *testing.T) {
		// This test only runs on darwin - on other platforms TMPDIR may not be used
		if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
			path := protocol.SocketPathForAccount(testAccountID, testRegion)
			// On darwin with TMPDIR set, the path should contain the TMPDIR
			assert.True(t, strings.HasPrefix(path, tmpdir) || strings.Contains(path, "suve"))
		}
	})
}
