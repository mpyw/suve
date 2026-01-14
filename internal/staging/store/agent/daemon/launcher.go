// Package daemon provides daemon lifecycle management for the staging agent.
package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/internal/ipc"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

const (
	connectTimeout = 5 * time.Second
	retryDelay     = 100 * time.Millisecond
)

// processSpawner is an interface for spawning daemon processes.
// This allows mocking in tests.
type processSpawner interface {
	Spawn(accountID, region string) error
}

// defaultProcessSpawner spawns daemon processes using exec.Command.
type defaultProcessSpawner struct{}

func (s *defaultProcessSpawner) Spawn(accountID, region string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	//nolint:gosec,noctx // G204: executable is from os.Executable(), not user input; noctx: intentionally no context for background daemon
	cmd := exec.Command(executable, "stage", "agent", "start", "--foreground", "--account", accountID, "--region", region)
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

// LauncherOption configures a Launcher.
type LauncherOption func(*Launcher)

// WithAutoStartDisabled disables automatic daemon startup.
func WithAutoStartDisabled() LauncherOption {
	return func(l *Launcher) {
		l.autoStartDisabled = true
	}
}

// withSpawner is an internal option to set a custom process spawner (for testing).
func withSpawner(spawner processSpawner) LauncherOption {
	return func(l *Launcher) {
		l.spawner = spawner
	}
}

// Launcher manages daemon startup and connectivity.
type Launcher struct {
	accountID         string
	region            string
	client            *ipc.Client
	spawner           processSpawner
	autoStartDisabled bool
}

// NewLauncher creates a new daemon launcher for a specific AWS account and region.
func NewLauncher(accountID, region string, opts ...LauncherOption) *Launcher {
	l := &Launcher{
		accountID: accountID,
		region:    region,
		client:    ipc.NewClient(accountID, region),
		spawner:   &defaultProcessSpawner{},
	}
	for _, opt := range opts {
		opt(l)
	}

	return l
}

// SendRequest sends a request to the daemon.
func (l *Launcher) SendRequest(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	return l.client.SendRequest(ctx, req)
}

// Ping checks if the daemon is reachable.
func (l *Launcher) Ping(ctx context.Context) error {
	return l.client.Ping(ctx)
}

// EnsureRunning ensures the daemon is running, starting it if necessary.
func (l *Launcher) EnsureRunning(ctx context.Context) error {
	// Try to ping first
	if err := l.client.Ping(ctx); err == nil {
		return nil
	}

	// Start daemon
	if err := l.startProcess(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Wait for daemon to be ready
	ticker := time.NewTicker(retryDelay)
	defer ticker.Stop()

	deadline := time.Now().Add(connectTimeout)
	for time.Now().Before(deadline) {
		if err := l.client.Ping(ctx); err == nil {
			output.Info(os.Stderr, "staging agent started for account %s (%s)", l.accountID, l.region)

			return nil
		}

		<-ticker.C
	}

	return fmt.Errorf("daemon did not start within timeout")
}

// Shutdown sends a shutdown request to the daemon.
func (l *Launcher) Shutdown(ctx context.Context) error {
	resp, err := l.client.SendRequest(ctx, &protocol.Request{Method: protocol.MethodShutdown})
	if err != nil {
		return err
	}

	return resp.Err()
}

// startProcess starts a new daemon process.
func (l *Launcher) startProcess() error {
	if l.autoStartDisabled {
		return fmt.Errorf(
			"daemon not running and auto-start is disabled; "+
				"run 'suve stage agent start --account %s --region %s' manually",
			l.accountID, l.region,
		)
	}

	return l.spawner.Spawn(l.accountID, l.region)
}
