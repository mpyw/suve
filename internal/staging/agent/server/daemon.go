// Package server provides the staging agent daemon.
package server

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/mpyw/suve/internal/staging/agent/protocol"
	"github.com/mpyw/suve/internal/staging/agent/transport"
)

// DaemonOption configures a Daemon.
type DaemonOption func(*Daemon)

// WithAutoShutdownDisabled disables automatic shutdown when state becomes empty.
func WithAutoShutdownDisabled() DaemonOption {
	return func(d *Daemon) {
		d.autoShutdownDisabled = true
	}
}

// Daemon represents the staging agent daemon.
type Daemon struct {
	server               *transport.Server
	state                *secureState
	autoShutdownDisabled bool
}

// NewDaemon creates a new daemon instance.
// Uses context.Background() intentionally: the daemon runs independently of the
// CLI command that started it and manages its own lifecycle via OS signals
// (SIGTERM/SIGINT) rather than parent context cancellation.
func NewDaemon(opts ...DaemonOption) *Daemon {
	d := &Daemon{
		state: newSecureState(),
	}
	for _, opt := range opts {
		opt(d)
	}
	d.server = transport.NewServer(d.handleRequest)
	d.server.OnResponse = d.onResponse
	return d
}

// Run starts the daemon and blocks until shutdown.
func (d *Daemon) Run() error {
	if err := d.server.Start(); err != nil {
		return err
	}

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			d.Shutdown()
		case <-d.server.Done():
		}
	}()

	d.server.Serve()
	return nil
}

// Shutdown gracefully shuts down the daemon.
func (d *Daemon) Shutdown() {
	d.server.Shutdown()
	d.state.destroy()
}

// onResponse is called after each response to check for auto-shutdown.
func (d *Daemon) onResponse(req *protocol.Request, resp *protocol.Response) {
	// Check for auto-shutdown after UnstageEntry, UnstageTag, or UnstageAll
	if !d.autoShutdownDisabled && resp.Success {
		switch req.Method {
		case protocol.MethodUnstageEntry, protocol.MethodUnstageTag, protocol.MethodUnstageAll:
			if d.state.isEmpty() {
				go d.Shutdown()
			}
		}
	}
}

// handleRequest processes a request and returns a response.
func (d *Daemon) handleRequest(req *protocol.Request) *protocol.Response {
	switch req.Method {
	case protocol.MethodPing:
		return d.handlePing()
	case protocol.MethodShutdown:
		go d.Shutdown()
		return successResponse()
	case protocol.MethodGetEntry:
		return d.handleGetEntry(req)
	case protocol.MethodGetTag:
		return d.handleGetTag(req)
	case protocol.MethodListEntries:
		return d.handleListEntries(req)
	case protocol.MethodListTags:
		return d.handleListTags(req)
	case protocol.MethodLoad:
		return d.handleLoad(req)
	case protocol.MethodStageEntry:
		return d.handleStageEntry(req)
	case protocol.MethodStageTag:
		return d.handleStageTag(req)
	case protocol.MethodUnstageEntry:
		return d.handleUnstageEntry(req)
	case protocol.MethodUnstageTag:
		return d.handleUnstageTag(req)
	case protocol.MethodUnstageAll:
		return d.handleUnstageAll(req)
	case protocol.MethodGetState:
		return d.handleGetState(req)
	case protocol.MethodSetState:
		return d.handleSetState(req)
	case protocol.MethodIsEmpty:
		return d.handleIsEmpty()
	default:
		return errorMessageResponse("unknown method: " + string(req.Method))
	}
}
