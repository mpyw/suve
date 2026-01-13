package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/internal/ipc"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/server"
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
	accountID            string
	region               string
	server               *ipc.Server
	handler              *server.Handler
	autoShutdownDisabled bool
}

// NewRunner creates a new daemon runner for a specific AWS account and region.
func NewRunner(accountID, region string, opts ...RunnerOption) *Runner {
	r := &Runner{
		accountID: accountID,
		region:    region,
		handler:   server.NewHandler(),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.server = ipc.NewServer(accountID, region, r.handler.HandleRequest, r.handleAutoShutdown)
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
				output.Printf(os.Stderr, "info: staging agent stopped (no staged changes)\n")
				go r.Shutdown()
			}
		}
	}
}
