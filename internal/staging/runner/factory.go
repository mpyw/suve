package runner

import (
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent"
	"github.com/mpyw/suve/internal/staging/agent/client"
	"github.com/mpyw/suve/internal/staging/agent/transport"
)

// NewStore creates a StoreReadWriteOperator using the agent daemon.
// The agent daemon is started automatically if not running, unless
// manual mode is enabled (see [agent.EnvDaemonAutoStart]).
func NewStore(accountID, region string, opts ...transport.ClientOption) staging.StoreReadWriteOperator {
	opts = append(agent.ClientOptions(), opts...)
	return client.NewStore(accountID, region, opts...)
}
