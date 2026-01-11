package agent

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

	"github.com/mpyw/suve/internal/staging"
)

const (
	connectTimeout = 5 * time.Second
	requestTimeout = 1 * time.Second
	retryDelay     = 100 * time.Millisecond
)

// ErrDaemonNotRunning is returned when the daemon is not running.
var ErrDaemonNotRunning = errors.New("daemon not running")

// Client provides communication with the staging agent daemon.
type Client struct {
	socketPath string
	mu         sync.Mutex
}

// NewClient creates a new client.
func NewClient() *Client {
	return &Client{
		socketPath: getSocketPath(),
	}
}

// ensureDaemon ensures the daemon is running, starting it if necessary.
func (c *Client) ensureDaemon(ctx context.Context) error {
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
	// Get the current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Start daemon as background process
	cmd := exec.Command(executable, "stage", "agent", "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Detach from current process group
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	// Release the process so it continues running after we exit
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("failed to release daemon process: %w", err)
	}

	return nil
}

// ping checks if the daemon is running.
func (c *Client) ping(ctx context.Context) error {
	return c.doSimpleRequest(ctx, &Request{Method: MethodPing})
}

// Shutdown sends a shutdown request to the daemon.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.doSimpleRequest(ctx, &Request{Method: MethodShutdown})
}

// IsEmpty checks if the daemon state is empty.
func (c *Client) IsEmpty(ctx context.Context) (bool, error) {
	return doRequestWithResult(c, ctx, &Request{Method: MethodIsEmpty}, func(r *IsEmptyResponse) bool { return r.Empty })
}

// GetEntry retrieves a staged entry from the daemon.
func (c *Client) GetEntry(ctx context.Context, accountID, region string, service staging.Service, name string) (*staging.Entry, error) {
	return doRequestWithResultEnsuringDaemon(c, ctx, &Request{
		Method:    MethodGetEntry,
		AccountID: accountID,
		Region:    region,
		Service:   service,
		Name:      name,
	}, func(r *EntryResponse) *staging.Entry { return r.Entry })
}

// GetTag retrieves staged tag changes from the daemon.
func (c *Client) GetTag(ctx context.Context, accountID, region string, service staging.Service, name string) (*staging.TagEntry, error) {
	return doRequestWithResultEnsuringDaemon(c, ctx, &Request{
		Method:    MethodGetTag,
		AccountID: accountID,
		Region:    region,
		Service:   service,
		Name:      name,
	}, func(r *TagResponse) *staging.TagEntry { return r.TagEntry })
}

// ListEntries returns all staged entries from the daemon.
func (c *Client) ListEntries(ctx context.Context, accountID, region string, service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
	return doRequestWithResultEnsuringDaemon(c, ctx, &Request{
		Method:    MethodListEntries,
		AccountID: accountID,
		Region:    region,
		Service:   service,
	}, func(r *ListEntriesResponse) map[staging.Service]map[string]staging.Entry { return r.Entries })
}

// ListTags returns all staged tag changes from the daemon.
func (c *Client) ListTags(ctx context.Context, accountID, region string, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	return doRequestWithResultEnsuringDaemon(c, ctx, &Request{
		Method:    MethodListTags,
		AccountID: accountID,
		Region:    region,
		Service:   service,
	}, func(r *ListTagsResponse) map[staging.Service]map[string]staging.TagEntry { return r.Tags })
}

// Load loads the full state from the daemon.
func (c *Client) Load(ctx context.Context, accountID, region string) (*staging.State, error) {
	return doRequestWithResultEnsuringDaemon(c, ctx, &Request{
		Method:    MethodLoad,
		AccountID: accountID,
		Region:    region,
	}, func(r *StateResponse) *staging.State { return r.State })
}

// StageEntry adds or updates a staged entry in the daemon.
func (c *Client) StageEntry(ctx context.Context, accountID, region string, service staging.Service, name string, entry staging.Entry) error {
	if err := c.ensureDaemon(ctx); err != nil {
		return err
	}
	return c.doSimpleRequest(ctx, &Request{
		Method:    MethodStageEntry,
		AccountID: accountID,
		Region:    region,
		Service:   service,
		Name:      name,
		Entry:     &entry,
	})
}

// StageTag adds or updates staged tag changes in the daemon.
func (c *Client) StageTag(ctx context.Context, accountID, region string, service staging.Service, name string, tagEntry staging.TagEntry) error {
	if err := c.ensureDaemon(ctx); err != nil {
		return err
	}
	return c.doSimpleRequest(ctx, &Request{
		Method:    MethodStageTag,
		AccountID: accountID,
		Region:    region,
		Service:   service,
		Name:      name,
		TagEntry:  &tagEntry,
	})
}

// UnstageEntry removes a staged entry from the daemon.
func (c *Client) UnstageEntry(ctx context.Context, accountID, region string, service staging.Service, name string) error {
	if err := c.ensureDaemon(ctx); err != nil {
		return err
	}
	return c.doSimpleRequest(ctx, &Request{
		Method:    MethodUnstageEntry,
		AccountID: accountID,
		Region:    region,
		Service:   service,
		Name:      name,
	})
}

// UnstageTag removes staged tag changes from the daemon.
func (c *Client) UnstageTag(ctx context.Context, accountID, region string, service staging.Service, name string) error {
	if err := c.ensureDaemon(ctx); err != nil {
		return err
	}
	return c.doSimpleRequest(ctx, &Request{
		Method:    MethodUnstageTag,
		AccountID: accountID,
		Region:    region,
		Service:   service,
		Name:      name,
	})
}

// UnstageAll removes all staged changes from the daemon.
func (c *Client) UnstageAll(ctx context.Context, accountID, region string, service staging.Service) error {
	if err := c.ensureDaemon(ctx); err != nil {
		return err
	}
	return c.doSimpleRequest(ctx, &Request{
		Method:    MethodUnstageAll,
		AccountID: accountID,
		Region:    region,
		Service:   service,
	})
}

// GetState retrieves the full state for persist operations.
func (c *Client) GetState(ctx context.Context, accountID, region string) (*staging.State, error) {
	return doRequestWithResult(c, ctx, &Request{
		Method:    MethodGetState,
		AccountID: accountID,
		Region:    region,
	}, func(r *StateResponse) *staging.State { return r.State })
}

// SetState sets the full state for drain operations.
func (c *Client) SetState(ctx context.Context, accountID, region string, state *staging.State) error {
	if err := c.ensureDaemon(ctx); err != nil {
		return err
	}

	resp, err := c.sendRequest(ctx, &Request{
		Method:    MethodSetState,
		AccountID: accountID,
		Region:    region,
		State:     state,
	})
	if err != nil {
		return err
	}
	return resp.Err()
}

// doRequestWithResult sends a request to the daemon and unmarshals the response.
func doRequestWithResult[Resp any, Result any](
	c *Client,
	ctx context.Context,
	req *Request,
	extract func(*Resp) Result,
) (Result, error) {
	var zero Result

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return zero, err
	}
	if err := resp.Err(); err != nil {
		return zero, err
	}

	var result Resp
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return extract(&result), nil
}

// doRequestWithResultEnsuringDaemon ensures the daemon is running, then sends a request and unmarshals the response.
func doRequestWithResultEnsuringDaemon[Resp any, Result any](
	c *Client,
	ctx context.Context,
	req *Request,
	extract func(*Resp) Result,
) (Result, error) {
	var zero Result
	if err := c.ensureDaemon(ctx); err != nil {
		return zero, err
	}
	return doRequestWithResult(c, ctx, req, extract)
}

// sendRequest sends a request to the daemon and returns the response.
func (c *Client) sendRequest(ctx context.Context, req *Request) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connect with timeout
	dialer := net.Dialer{Timeout: requestTimeout}
	conn, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDaemonNotRunning, err)
	}
	defer func() { _ = conn.Close() }()

	// Set deadline
	if err := conn.SetDeadline(time.Now().Add(requestTimeout)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}

	// Send request
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(conn)
	var resp Response
	if err := decoder.Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("daemon closed connection unexpectedly")
		}
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &resp, nil
}

// doSimpleRequest sends a request and returns only the error status.
func (c *Client) doSimpleRequest(ctx context.Context, req *Request) error {
	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return err
	}
	return resp.Err()
}
