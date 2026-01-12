package runner

import (
	"os"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/client"
)

// NewStore creates a StoreReadWriteOperator using the agent daemon.
// The agent daemon is started automatically if not running, unless
// SUVE_DAEMON_AUTO_START=0 is set or opts includes WithAutoStartDisabled.
func NewStore(accountID, region string, opts ...client.ClientOption) staging.StoreReadWriteOperator {
	// Check environment variable for auto-start setting
	if os.Getenv("SUVE_DAEMON_AUTO_START") == "0" {
		opts = append(opts, client.WithAutoStartDisabled())
	}
	return client.NewStore(accountID, region, opts...)
}
