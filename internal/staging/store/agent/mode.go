// Package agent provides shared types and configuration for the staging agent
// client and server packages.
package agent

import (
	"os"

	"github.com/mpyw/suve/internal/staging/store/agent/daemon"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/client"
)

// Environment variable names for agent configuration.
const (
	// EnvDaemonAutoStart controls automatic daemon startup and shutdown.
	// Set to "0" to disable both auto-start and auto-shutdown (manual mode).
	EnvDaemonAutoStart = "SUVE_DAEMON_AUTO_START"
)

// isManualMode returns true if the daemon should be managed manually.
// When true, both auto-start and auto-shutdown are disabled.
func isManualMode() bool {
	return os.Getenv(EnvDaemonAutoStart) == "0"
}

// ClientOptions returns client options based on the current mode.
func ClientOptions() []client.StoreOption {
	if isManualMode() {
		return []client.StoreOption{client.WithAutoStartDisabled()}
	}
	return nil
}

// DaemonOptions returns daemon options based on the current mode.
func DaemonOptions() []daemon.RunnerOption {
	if isManualMode() {
		return []daemon.RunnerOption{daemon.WithAutoShutdownDisabled()}
	}
	return nil
}
