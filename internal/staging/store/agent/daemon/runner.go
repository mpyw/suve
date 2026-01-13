package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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
	cancel               context.CancelFunc
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
	r.server = ipc.NewServer(accountID, region, r.handler.HandleRequest, r.checkAutoShutdown, r.Shutdown)
	return r
}

// Run starts the daemon and blocks until shutdown.
// The provided context is used for graceful shutdown - when cancelled, the daemon will stop.
// Additionally, SIGTERM and SIGINT signals will trigger shutdown.
func (r *Runner) Run(ctx context.Context) error {
	if err := r.server.Start(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			r.Shutdown()
		case <-ctx.Done():
			// Context cancelled, no need to do anything
		}
	}()

	r.server.Serve(ctx)
	return nil
}

// Shutdown gracefully shuts down the daemon.
func (r *Runner) Shutdown() {
	if r.cancel != nil {
		r.cancel()
	}
	r.handler.Destroy()
}

// checkAutoShutdown sets WillShutdown flag if the daemon should shut down after this response.
// The actual shutdown is triggered by the IPC server after the response is sent.
func (r *Runner) checkAutoShutdown(req *protocol.Request, resp *protocol.Response) {
	if !r.autoShutdownDisabled && resp.Success {
		switch req.Method {
		case protocol.MethodUnstageEntry, protocol.MethodUnstageTag:
			if r.handler.IsEmpty() {
				resp.WillShutdown = true
				// Use hint to determine reason, default to "empty"
				switch req.Hint {
				case protocol.HintApply:
					resp.ShutdownReason = protocol.ShutdownReasonApplied
				case protocol.HintReset:
					resp.ShutdownReason = protocol.ShutdownReasonUnstaged
				case protocol.HintPersist:
					resp.ShutdownReason = protocol.ShutdownReasonPersisted
				default:
					resp.ShutdownReason = protocol.ShutdownReasonEmpty
				}
			}
		case protocol.MethodUnstageAll:
			if r.handler.IsEmpty() {
				resp.WillShutdown = true
				// UnstageAll is typically from reset or persist, use hint
				switch req.Hint {
				case protocol.HintApply:
					resp.ShutdownReason = protocol.ShutdownReasonApplied
				case protocol.HintPersist:
					resp.ShutdownReason = protocol.ShutdownReasonPersisted
				default:
					resp.ShutdownReason = protocol.ShutdownReasonUnstaged
				}
			}
		case protocol.MethodSetState:
			if r.handler.IsEmpty() {
				resp.WillShutdown = true
				resp.ShutdownReason = protocol.ShutdownReasonCleared
			}
		}
	}
}
