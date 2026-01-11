package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Daemon represents the staging agent daemon.
type Daemon struct {
	socketPath string
	listener   net.Listener
	state      *secureState
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	shutdownCh chan struct{}
}

// NewDaemon creates a new daemon instance.
// Uses context.Background() intentionally: the daemon runs independently of the
// CLI command that started it and manages its own lifecycle via OS signals
// (SIGTERM/SIGINT) rather than parent context cancellation.
func NewDaemon() *Daemon {
	ctx, cancel := context.WithCancel(context.Background())
	return &Daemon{
		socketPath: getSocketPath(),
		state:      newSecureState(),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
	}
}

// Run starts the daemon and blocks until shutdown.
func (d *Daemon) Run() error {
	// Setup process security
	if err := setupProcessSecurity(); err != nil {
		return fmt.Errorf("failed to setup process security: %w", err)
	}

	// Create socket directory
	if err := createSocketDir(d.socketPath); err != nil {
		return err
	}

	// Remove existing socket
	if err := removeExistingSocket(d.socketPath); err != nil {
		return err
	}

	// Listen on Unix socket
	listener, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	d.listener = listener

	// Set socket permissions
	if err := setSocketPermissions(d.socketPath); err != nil {
		_ = listener.Close()
		return err
	}

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			d.Shutdown()
		case <-d.ctx.Done():
		}
	}()

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-d.ctx.Done():
				return nil
			default:
				// Log error and continue
				continue
			}
		}

		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleConnection(conn)
		}()
	}
}

// Shutdown gracefully shuts down the daemon.
func (d *Daemon) Shutdown() {
	d.cancel()
	if d.listener != nil {
		_ = d.listener.Close()
	}
	d.wg.Wait()
	d.state.destroy()
	close(d.shutdownCh)
}

// handleConnection handles a single client connection.
func (d *Daemon) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Set read deadline
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Verify peer credentials
	if err := verifyPeerCredentials(conn); err != nil {
		d.sendError(conn, err.Error())
		return
	}

	// Read request
	decoder := json.NewDecoder(conn)
	var req Request
	if err := decoder.Decode(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			d.sendError(conn, fmt.Sprintf("failed to decode request: %v", err))
		}
		return
	}

	// Handle request
	resp := d.handleRequest(&req)

	// Send response
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)

	// Check for auto-shutdown after UnstageEntry, UnstageTag, or UnstageAll
	if resp.Success && (req.Method == MethodUnstageEntry || req.Method == MethodUnstageTag || req.Method == MethodUnstageAll) {
		if d.state.isEmpty() {
			// Schedule shutdown
			go d.Shutdown()
		}
	}
}

// handleRequest processes a request and returns a response.
func (d *Daemon) handleRequest(req *Request) *Response {
	switch req.Method {
	case MethodPing:
		return d.handlePing()
	case MethodShutdown:
		go d.Shutdown()
		return &Response{Success: true}
	case MethodGetEntry:
		return d.handleGetEntry(req)
	case MethodGetTag:
		return d.handleGetTag(req)
	case MethodListEntries:
		return d.handleListEntries(req)
	case MethodListTags:
		return d.handleListTags(req)
	case MethodLoad:
		return d.handleLoad(req)
	case MethodStageEntry:
		return d.handleStageEntry(req)
	case MethodStageTag:
		return d.handleStageTag(req)
	case MethodUnstageEntry:
		return d.handleUnstageEntry(req)
	case MethodUnstageTag:
		return d.handleUnstageTag(req)
	case MethodUnstageAll:
		return d.handleUnstageAll(req)
	case MethodGetState:
		return d.handleGetState(req)
	case MethodSetState:
		return d.handleSetState(req)
	case MethodIsEmpty:
		return d.handleIsEmpty()
	default:
		return &Response{Success: false, Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

// sendError sends an error response.
func (d *Daemon) sendError(conn net.Conn, msg string) {
	resp := Response{Success: false, Error: msg}
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)
}

// GetSocketPath returns the socket path for clients.
func GetSocketPath() string {
	return getSocketPath()
}
