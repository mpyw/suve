package agent

import (
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/client"
)

// StoreOption configures a Store.
type StoreOption = client.StoreOption

// NewStore creates an AgentStore using the agent daemon.
// The agent daemon is started automatically if not running, unless
// manual mode is enabled (see [EnvDaemonManualMode]).
func NewStore(scope staging.Scope, opts ...StoreOption) store.AgentStore {
	opts = append(ClientOptions(), opts...)

	return client.NewStore(scope, opts...)
}
