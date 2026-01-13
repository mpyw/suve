package agent

import (
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/client"
)

// NewStore creates a StoreReadWriteOperator using the agent daemon.
// The agent daemon is started automatically if not running, unless
// manual mode is enabled (see [EnvDaemonAutoStart]).
func NewStore(accountID, region string, opts ...client.StoreOption) staging.StoreReadWriteOperator {
	opts = append(ClientOptions(), opts...)
	return client.NewStore(accountID, region, opts...)
}
