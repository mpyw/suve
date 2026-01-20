// Package file provides file-based staging storage.
// This package implements StateIO interface for drain/persist commands.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

const (
	paramFileName  = "param.json"
	secretFileName = "secret.json"
	baseDirName    = ".suve"
	stagingDir     = "staging"
)

// fileMu protects concurrent access to the state files within a process.
//
//nolint:gochecknoglobals // process-wide mutex for file access synchronization
var fileMu sync.Mutex

// Hooks for testing - these allow tests to inject errors.
//
//nolint:gochecknoglobals // test hook for dependency injection
var userHomeDirFunc = os.UserHomeDir

// Store manages the staging state using the filesystem.
// It implements StateIO interface for drain/persist operations.
// State is split into param.json and secret.json files.
type Store struct {
	stateDir   string
	passphrase string
}

// NewStore creates a new file Store with the default state directory.
// The state files are stored under ~/.suve/staging/{scope.Key()}/
// with param.json and secret.json for respective services.
func NewStore(scope staging.Scope) (*Store, error) {
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, baseDirName, stagingDir, scope.Key())

	return &Store{
		stateDir: stateDir,
	}, nil
}

// NewStoreWithDir creates a new file Store with a custom state directory.
// This is primarily for testing.
func NewStoreWithDir(dir string) *Store {
	return &Store{
		stateDir: dir,
	}
}

// NewStoreWithPassphrase creates a new file Store with a passphrase for encryption.
// This is used by drain/persist commands that need StateIO interface.
func NewStoreWithPassphrase(scope staging.Scope, passphrase string) (*Store, error) {
	s, err := NewStore(scope)
	if err != nil {
		return nil, err
	}

	s.passphrase = passphrase

	return s, nil
}

// SetPassphrase sets the passphrase for encryption/decryption.
// This is primarily for testing.
func (s *Store) SetPassphrase(passphrase string) {
	s.passphrase = passphrase
}

// paramPath returns the path to the param.json file.
func (s *Store) paramPath() string {
	return filepath.Join(s.stateDir, paramFileName)
}

// secretPath returns the path to the secret.json file.
func (s *Store) secretPath() string {
	return filepath.Join(s.stateDir, secretFileName)
}

// pathForService returns the file path for the given service.
func (s *Store) pathForService(service staging.Service) string {
	switch service {
	case staging.ServiceParam:
		return s.paramPath()
	case staging.ServiceSecret:
		return s.secretPath()
	default:
		return ""
	}
}

// Exists checks if any state file exists.
func (s *Store) Exists() (bool, error) {
	paramExists, err := fileExists(s.paramPath())
	if err != nil {
		return false, err
	}

	if paramExists {
		return true, nil
	}

	return fileExists(s.secretPath())
}

// fileExists checks if a file exists.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to check file: %w", err)
	}

	return true, nil
}

// IsEncrypted checks if any stored file is encrypted.
// Returns true if at least one file exists and is encrypted.
func (s *Store) IsEncrypted() (bool, error) {
	// Check param file
	paramEncrypted, err := isFileEncrypted(s.paramPath())
	if err != nil {
		return false, err
	}

	if paramEncrypted {
		return true, nil
	}

	// Check secret file
	return isFileEncrypted(s.secretPath())
}

// isFileEncrypted checks if a specific file is encrypted.
func isFileEncrypted(path string) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from internal methods, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to read file: %w", err)
	}

	return crypt.IsEncrypted(data), nil
}

// Drain reads the state from file(s), optionally deleting the file(s).
// This implements StateDrainer for file-based storage.
// If service is empty, returns all services; otherwise filters to the specified service.
// If keep is false, the file(s) is deleted after reading.
func (s *Store) Drain(_ context.Context, service staging.Service, keep bool) (*staging.State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	if service != "" {
		// Read specific service file
		return s.drainService(service, keep)
	}

	// Read both files and merge
	paramState, err := s.drainService(staging.ServiceParam, keep)
	if err != nil {
		return nil, err
	}

	secretState, err := s.drainService(staging.ServiceSecret, keep)
	if err != nil {
		return nil, err
	}

	// Merge states
	merged := staging.NewEmptyState()
	merged.Merge(paramState)
	merged.Merge(secretState)

	return merged, nil
}

// drainService reads state for a specific service.
// Must be called with fileMu held.
func (s *Store) drainService(service staging.Service, keep bool) (*staging.State, error) {
	path := s.pathForService(service)
	if path == "" {
		return staging.NewEmptyState(), nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is from pathForService, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return staging.NewEmptyState(), nil
		}

		return nil, fmt.Errorf("failed to read %s file: %w", service, err)
	}

	// Decrypt if encrypted
	if crypt.IsEncrypted(data) {
		if s.passphrase == "" {
			return nil, crypt.ErrDecryptionFailed
		}

		data, err = crypt.Decrypt(data, s.passphrase)
		if err != nil {
			return nil, err
		}
	}

	var state staging.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse %s file: %w", service, err)
	}

	// Initialize maps if nil
	initializeStateMaps(&state)

	// Delete file if keep is false
	if !keep {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove %s file: %w", service, err)
		}
	}

	// Return only the requested service's data
	return state.ExtractService(service), nil
}

// WriteState saves the state to file(s).
// This implements StateWriter for file-based storage.
// If service is empty, writes to both files; otherwise writes only to the specified service's file.
func (s *Store) WriteState(_ context.Context, service staging.Service, state *staging.State) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(s.stateDir, 0o700); err != nil { //nolint:mnd // owner-only directory permissions
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	if service != "" {
		// Write specific service file
		return s.writeService(service, state.ExtractService(service))
	}

	// Write both files
	var err error

	if e := s.writeService(staging.ServiceParam, state.ExtractService(staging.ServiceParam)); e != nil {
		err = errors.Join(err, fmt.Errorf("param: %w", e))
	}

	if e := s.writeService(staging.ServiceSecret, state.ExtractService(staging.ServiceSecret)); e != nil {
		err = errors.Join(err, fmt.Errorf("secret: %w", e))
	}

	return err
}

// writeService writes state for a specific service.
// Must be called with fileMu held.
func (s *Store) writeService(service staging.Service, state *staging.State) error {
	path := s.pathForService(service)
	if path == "" {
		return nil
	}

	// Check if there are any staged changes for this service
	serviceState := state.ExtractService(service)
	if serviceState.IsEmpty() {
		// Remove file if no staged changes
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty %s file: %w", service, err)
		}

		return nil
	}

	data, err := json.MarshalIndent(serviceState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s state: %w", service, err)
	}

	// Encrypt if passphrase is provided
	if s.passphrase != "" {
		data, err = crypt.Encrypt(data, s.passphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt %s state: %w", service, err)
		}
	}

	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:mnd // owner-only file permissions
		return fmt.Errorf("failed to write %s file: %w", service, err)
	}

	return nil
}

// initializeStateMaps ensures all nested maps are initialized.
func initializeStateMaps(state *staging.State) {
	if state.Entries == nil {
		state.Entries = make(map[staging.Service]map[string]staging.Entry)
	}

	if state.Entries[staging.ServiceParam] == nil {
		state.Entries[staging.ServiceParam] = make(map[string]staging.Entry)
	}

	if state.Entries[staging.ServiceSecret] == nil {
		state.Entries[staging.ServiceSecret] = make(map[string]staging.Entry)
	}

	if state.Tags == nil {
		state.Tags = make(map[staging.Service]map[string]staging.TagEntry)
	}

	if state.Tags[staging.ServiceParam] == nil {
		state.Tags[staging.ServiceParam] = make(map[string]staging.TagEntry)
	}

	if state.Tags[staging.ServiceSecret] == nil {
		state.Tags[staging.ServiceSecret] = make(map[string]staging.TagEntry)
	}
}

// Delete removes all state files without reading their contents.
// This is useful for dropping stash when decryption is not needed.
func (s *Store) Delete() error {
	fileMu.Lock()
	defer fileMu.Unlock()

	var err error

	if e := os.Remove(s.paramPath()); e != nil && !os.IsNotExist(e) {
		err = errors.Join(err, fmt.Errorf("failed to remove param file: %w", e))
	}

	if e := os.Remove(s.secretPath()); e != nil && !os.IsNotExist(e) {
		err = errors.Join(err, fmt.Errorf("failed to remove secret file: %w", e))
	}

	return err
}

// Compile-time check that Store implements FileStore.
var _ store.FileStore = (*Store)(nil)
