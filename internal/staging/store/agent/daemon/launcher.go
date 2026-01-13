// Package daemon provides daemon lifecycle management for the staging agent.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mpyw/suve/internal/staging/store/agent/daemon/internal/ipc"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

const (
	connectTimeout = 5 * time.Second
	retryDelay     = 100 * time.Millisecond
)

// LauncherOption configures a Launcher.
type LauncherOption func(*Launcher)

// WithAutoStartDisabled disables automatic daemon startup.
func WithAutoStartDisabled() LauncherOption {
	return func(l *Launcher) {
		l.autoStartDisabled = true
	}
}

// Launcher manages daemon startup and connectivity.
type Launcher struct {
	client            *ipc.Client
	autoStartDisabled bool
}

// NewLauncher creates a new daemon launcher.
func NewLauncher(opts ...LauncherOption) *Launcher {
	l := &Launcher{
		client: ipc.NewClient(),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// SendRequest sends a request to the daemon.
func (l *Launcher) SendRequest(req *protocol.Request) (*protocol.Response, error) {
	return l.client.SendRequest(req)
}

// Ping checks if the daemon is reachable.
func (l *Launcher) Ping() error {
	return l.client.Ping()
}

// EnsureRunning ensures the daemon is running, starting it if necessary.
func (l *Launcher) EnsureRunning() error {
	// Try to ping first
	if err := l.client.Ping(); err == nil {
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
		if err := l.client.Ping(); err == nil {
			return nil
		}
		<-ticker.C
	}

	return fmt.Errorf("daemon did not start within timeout")
}

// Shutdown sends a shutdown request to the daemon.
func (l *Launcher) Shutdown() error {
	resp, err := l.client.SendRequest(&protocol.Request{Method: protocol.MethodShutdown})
	if err != nil {
		return err
	}
	return resp.Err()
}

// startProcess starts a new daemon process.
func (l *Launcher) startProcess() error {
	if l.autoStartDisabled {
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
