// Package ipc provides low-level IPC communication for the staging agent.
package ipc

import (
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

// NewClient creates a new IPC client.
func NewClient() *Client {
	return &Client{
		socketPath: protocol.SocketPath(),
	}
}

// SendRequest sends a request to the daemon and returns the response.
func (c *Client) SendRequest(req *protocol.Request) (*protocol.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotConnected, err)
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

// Ping checks if the daemon is reachable.
func (c *Client) Ping() error {
	resp, err := c.SendRequest(&protocol.Request{Method: protocol.MethodPing})
	if err != nil {
		return err
	}
	return resp.Err()
}
