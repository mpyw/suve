package runner

import (
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/client"
)

// NewStore creates a StoreReadWriteOperator using the agent daemon.
// The agent daemon is started automatically if not running.
func NewStore(accountID, region string) staging.StoreReadWriteOperator {
	return client.NewStore(accountID, region)
}
