package transport

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

	"github.com/mpyw/suve/internal/staging/agent/protocol"
	"github.com/mpyw/suve/internal/staging/agent/server/security"
)

const (
	connectionTimeout = 5 * time.Second
)

// RequestHandler handles incoming requests and returns responses.
type RequestHandler func(*protocol.Request) *protocol.Response

// Server provides low-level IPC server functionality.
type Server struct {
	listener net.Listener
	handler  RequestHandler
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc

	// OnResponse is called after a response is sent, allowing post-processing.
	OnResponse func(*protocol.Request, *protocol.Response)
}

// NewServer creates a new transport server.
func NewServer(handler RequestHandler) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts listening on the Unix socket.
func (s *Server) Start() error {
	if err := security.SetupProcess(); err != nil {
		return fmt.Errorf("failed to setup process security: %w", err)
	}

	socketPath := protocol.SocketPath()

	if err := s.createSocketDir(socketPath); err != nil {
		return err
	}

	if err := s.removeExistingSocket(socketPath); err != nil {
		return err
	}

	listener, err := net.Listen("unix", socketPath)
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

// Serve accepts and handles connections until shutdown.
func (s *Server) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown() {
	s.cancel()
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
}

// Done returns a channel that's closed when the server is shutting down.
func (s *Server) Done() <-chan struct{} {
	return s.ctx.Done()
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

	_ = conn.SetDeadline(time.Now().Add(connectionTimeout))
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)

	if s.OnResponse != nil {
		s.OnResponse(&req, resp)
	}
}

// sendError sends an error response.
func (s *Server) sendError(conn net.Conn, msg string) {
	resp := protocol.Response{Success: false, Error: msg}
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)
}

// createSocketDir creates the socket directory with secure permissions.
func (s *Server) createSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
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
