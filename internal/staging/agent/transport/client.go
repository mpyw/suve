// Package transport provides low-level IPC communication for the staging agent.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/mpyw/suve/internal/staging/agent/protocol"
)

const (
	connectTimeout = 5 * time.Second
	requestTimeout = 1 * time.Second
	retryDelay     = 100 * time.Millisecond
)

// ErrDaemonNotRunning is returned when the daemon is not running.
var ErrDaemonNotRunning = errors.New("daemon not running")

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithAutoStartDisabled disables automatic daemon startup.
func WithAutoStartDisabled() ClientOption {
	return func(c *Client) {
		c.autoStartDisabled = true
	}
}

// Client provides low-level IPC communication with the daemon.
type Client struct {
	socketPath        string
	autoStartDisabled bool
	mu                sync.Mutex
}

// NewClient creates a new transport client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		socketPath: protocol.SocketPath(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// EnsureDaemon ensures the daemon is running, starting it if necessary.
func (c *Client) EnsureDaemon(ctx context.Context) error {
	// Try to ping first
	if err := c.ping(ctx); err == nil {
		return nil
	}

	// Start daemon
	if err := c.startDaemon(ctx); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Set up ticker for retries
	ticker := time.NewTicker(retryDelay)
	defer ticker.Stop()

	// Wait for daemon to be ready
	deadline := time.Now().Add(connectTimeout)
	for time.Now().Before(deadline) {
		if err := c.ping(ctx); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}

	return fmt.Errorf("daemon did not start within timeout")
}

// startDaemon starts a new daemon process.
func (c *Client) startDaemon(_ context.Context) error {
	if c.autoStartDisabled {
		return fmt.Errorf("daemon not running and auto-start is disabled; run 'suve stage agent start' manually")
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(executable, "stage", "agent", "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("failed to release daemon process: %w", err)
	}

	return nil
}

// ping checks if the daemon is running.
func (c *Client) ping(ctx context.Context) error {
	return c.DoSimpleRequest(ctx, &protocol.Request{Method: protocol.MethodPing})
}

// Shutdown sends a shutdown request to the daemon.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.DoSimpleRequest(ctx, &protocol.Request{Method: protocol.MethodShutdown})
}

// IsEmpty checks if the daemon state is empty.
func (c *Client) IsEmpty(ctx context.Context) (bool, error) {
	resp, err := c.SendRequest(ctx, &protocol.Request{Method: protocol.MethodIsEmpty})
	if err != nil {
		return false, err
	}
	if err := resp.Err(); err != nil {
		return false, err
	}

	var result protocol.IsEmptyResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return false, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return result.Empty, nil
}

// SendRequest sends a request to the daemon and returns the response.
func (c *Client) SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	dialer := net.Dialer{Timeout: requestTimeout}
	conn, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDaemonNotRunning, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(requestTimeout)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	decoder := json.NewDecoder(conn)
	var resp protocol.Response
	if err := decoder.Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("daemon closed connection unexpectedly")
		}
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &resp, nil
}

// DoSimpleRequest sends a request and returns only the error status.
func (c *Client) DoSimpleRequest(ctx context.Context, req *protocol.Request) error {
	resp, err := c.SendRequest(ctx, req)
	if err != nil {
		return err
	}
	return resp.Err()
}
