// Package ipc provides low-level IPC communication for the staging agent.
package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

const (
	dialTimeout    = 1 * time.Second
	requestTimeout = 1 * time.Second
)

// ErrNotConnected is returned when the daemon is not reachable.
var ErrNotConnected = errors.New("daemon not connected")

// Client provides low-level IPC communication with the daemon.
type Client struct {
	socketPath string
	mu         sync.Mutex
}

// NewClient creates a new IPC client for a specific AWS account and region.
func NewClient(accountID, region string) *Client {
	return &Client{
		socketPath: protocol.SocketPathForAccount(accountID, region),
	}
}

// SendRequest sends a request to the daemon and returns the response.
// The context is used for connection establishment and request timeout.
func (c *Client) SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Use context deadline if set, otherwise use default timeout.
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	dialer := net.Dialer{}

	conn, err := dialer.DialContext(dialCtx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotConnected, err)
	}

	defer func() { _ = conn.Close() }()

	// Set request deadline based on context or default timeout.
	deadline := time.Now().Add(requestTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	if err := conn.SetDeadline(deadline); err != nil {
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

// Ping checks if the daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.SendRequest(ctx, &protocol.Request{Method: protocol.MethodPing})
	if err != nil {
		return err
	}

	return resp.Err()
}
