package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

const (
	connectionTimeout = 5 * time.Second
)

// RequestHandler handles incoming requests and returns responses.
type RequestHandler func(*protocol.Request) *protocol.Response

// ResponseCallback is called before a response is sent.
// It can modify the response (e.g., set WillShutdown flag).
type ResponseCallback func(*protocol.Request, *protocol.Response)

// ShutdownCallback is called after a response with WillShutdown=true is sent.
type ShutdownCallback func()

// Server provides low-level IPC server functionality.
type Server struct {
	accountID  string
	region     string
	listener   net.Listener
	handler    RequestHandler
	onResponse ResponseCallback
	onShutdown ShutdownCallback
	wg         sync.WaitGroup
}

// NewServer creates a new IPC server for a specific AWS account and region.
func NewServer(accountID, region string, handler RequestHandler, onResponse ResponseCallback, onShutdown ShutdownCallback) *Server {
	return &Server{
		accountID:  accountID,
		region:     region,
		handler:    handler,
		onResponse: onResponse,
		onShutdown: onShutdown,
	}
}

// Start starts listening on the Unix socket.
func (s *Server) Start(ctx context.Context) error {
	if err := security.SetupProcess(); err != nil {
		return fmt.Errorf("failed to setup process security: %w", err)
	}

	socketPath := protocol.SocketPathForAccount(s.accountID, s.region)

	if err := s.createSocketDir(socketPath); err != nil {
		return err
	}

	if err := s.removeExistingSocket(socketPath); err != nil {
		return err
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	s.listener = listener

	if err := s.setSocketPermissions(socketPath); err != nil {
		_ = listener.Close()
		return err
	}

	return nil
}

// Serve accepts and handles connections until the context is cancelled.
func (s *Server) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		if s.listener != nil {
			_ = s.listener.Close()
		}
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return
			default:
				continue
			}
		}

		s.wg.Add(1)
		//goroutinectx:ignore goroutine // handleConnection uses connection timeout, not context cancellation
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

// handleConnection handles a single client connection.
func (s *Server) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(connectionTimeout))

	if err := security.VerifyPeerCredentials(conn); err != nil {
		s.sendError(conn, err.Error())
		return
	}

	decoder := json.NewDecoder(conn)
	var req protocol.Request
	if err := decoder.Decode(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			s.sendError(conn, fmt.Sprintf("failed to decode request: %v", err))
		}
		return
	}

	resp := s.handler(&req)

	// Call onResponse BEFORE encoding to allow setting WillShutdown flag
	if s.onResponse != nil {
		s.onResponse(&req, resp)
	}

	_ = conn.SetDeadline(time.Now().Add(connectionTimeout))
	encoder := json.NewEncoder(conn)
	//nolint:errchkjson // Response struct is safe for JSON encoding
	_ = encoder.Encode(resp)

	// Trigger shutdown callback AFTER response is sent
	if resp.WillShutdown && s.onShutdown != nil {
		go s.onShutdown()
	}
}

// sendError sends an error response.
func (s *Server) sendError(conn net.Conn, msg string) {
	resp := protocol.Response{Success: false, Error: msg}
	encoder := json.NewEncoder(conn)
	//nolint:errchkjson // Response struct is safe for JSON encoding
	_ = encoder.Encode(resp)
}

// createSocketDir creates the socket directory with secure permissions.
func (s *Server) createSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
	//nolint:gosec // G302: 0o700 is appropriate for directories (owner rwx only)
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("failed to set socket directory permissions: %w", err)
	}
	return nil
}

// removeExistingSocket removes any existing socket file.
func (s *Server) removeExistingSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}
	return nil
}

// setSocketPermissions sets secure permissions on the socket file.
func (s *Server) setSocketPermissions(socketPath string) error {
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}
	return nil
}
