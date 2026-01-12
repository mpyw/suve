// Package server provides the staging agent daemon.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/mpyw/suve/internal/staging/agent/protocol"
)

// Daemon represents the staging agent daemon.
type Daemon struct {
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
		state:      newSecureState(),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
	}
}

// Run starts the daemon and blocks until shutdown.
func (d *Daemon) Run() error {
	// Setup process security
	if err := d.setupProcessSecurity(); err != nil {
		return fmt.Errorf("failed to setup process security: %w", err)
	}

	socketPath := protocol.SocketPath()

	// Create socket directory
	if err := d.createSocketDir(socketPath); err != nil {
		return err
	}

	// Remove existing socket
	if err := d.removeExistingSocket(socketPath); err != nil {
		return err
	}

	// Listen on Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	d.listener = listener

	// Set socket permissions
	if err := d.setSocketPermissions(socketPath); err != nil {
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
	if err := d.verifyPeerCredentials(conn); err != nil {
		d.sendError(conn, err.Error())
		return
	}

	// Read request
	decoder := json.NewDecoder(conn)
	var req protocol.Request
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
	if resp.Success && (req.Method == protocol.MethodUnstageEntry || req.Method == protocol.MethodUnstageTag || req.Method == protocol.MethodUnstageAll) {
		if d.state.isEmpty() {
			// Schedule shutdown
			go d.Shutdown()
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
		return &protocol.Response{Success: true}
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
		return &protocol.Response{Success: false, Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

// sendError sends an error response.
func (d *Daemon) sendError(conn net.Conn, msg string) {
	resp := protocol.Response{Success: false, Error: msg}
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)
}

// createSocketDir creates the socket directory with secure permissions.
func (d *Daemon) createSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
	// Ensure directory permissions are correct
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("failed to set socket directory permissions: %w", err)
	}
	return nil
}

// removeExistingSocket removes any existing socket file.
func (d *Daemon) removeExistingSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}
	return nil
}

// setSocketPermissions sets secure permissions on the socket file.
func (d *Daemon) setSocketPermissions(socketPath string) error {
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}
	return nil
}
