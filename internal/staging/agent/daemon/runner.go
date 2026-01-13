package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/mpyw/suve/internal/staging/agent/ipc"
	"github.com/mpyw/suve/internal/staging/agent/protocol"
	"github.com/mpyw/suve/internal/staging/agent/server"
)

// RunnerOption configures a Runner.
type RunnerOption func(*Runner)

// WithAutoShutdownDisabled disables automatic shutdown when state becomes empty.
func WithAutoShutdownDisabled() RunnerOption {
	return func(r *Runner) {
		r.autoShutdownDisabled = true
	}
}

// Runner represents the staging agent daemon process.
type Runner struct {
	server               *ipc.Server
	handler              *server.Handler
	autoShutdownDisabled bool
}

// NewRunner creates a new daemon runner.
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		handler: server.NewHandler(),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.server = ipc.NewServer(r.handler.HandleRequest, r.handleAutoShutdown)
	return r
}

// Run starts the daemon and blocks until shutdown.
func (r *Runner) Run() error {
	if err := r.server.Start(); err != nil {
		return err
	}

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			r.Shutdown()
		case <-r.server.Done():
		}
	}()

	r.server.Serve()
	return nil
}

// Shutdown gracefully shuts down the daemon.
func (r *Runner) Shutdown() {
	r.server.Shutdown()
	r.handler.Destroy()
}

// handleAutoShutdown checks if the daemon should shut down after unstage operations.
func (r *Runner) handleAutoShutdown(req *protocol.Request, resp *protocol.Response) {
	if !r.autoShutdownDisabled && resp.Success {
		switch req.Method {
		case protocol.MethodUnstageEntry, protocol.MethodUnstageTag, protocol.MethodUnstageAll:
			if r.handler.IsEmpty() {
				go r.Shutdown()
			}
		}
	}
}
