// Package file provides file-based staging storage.
// This package implements StateIO interface for drain/persist commands.
package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

const (
	stateFileName = "stage.json"
	stateDirName  = ".suve"
)

// ErrDecryptionFailed is returned when attempting to read an encrypted file without a passphrase,
// or when the passphrase is incorrect.
var ErrDecryptionFailed = crypt.ErrDecryptionFailed

// fileMu protects concurrent access to the state file within a process.
//
//nolint:gochecknoglobals // process-wide mutex for file access synchronization
var fileMu sync.Mutex

// Hooks for testing - these allow tests to inject errors.
//
//nolint:gochecknoglobals // test hook for dependency injection
var userHomeDirFunc = os.UserHomeDir

// Store manages the staging state using the filesystem.
// It implements StateIO interface for drain/persist operations.
type Store struct {
	stateFilePath string
	passphrase    string
}

// NewStore creates a new file Store with the default state file path.
// The state file is stored under ~/.suve/{accountID}/{region}/stage.json
// to isolate staging state per AWS account and region.
func NewStore(accountID, region string) (*Store, error) {
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, stateDirName, accountID, region)

	return &Store{
		stateFilePath: filepath.Join(stateDir, stateFileName),
	}, nil
}

// NewStoreWithPath creates a new file Store with a custom state file path.
// This is primarily for testing.
func NewStoreWithPath(path string) *Store {
	return &Store{
		stateFilePath: path,
	}
}

// NewStoreWithPassphrase creates a new file Store with a passphrase for encryption.
// This is used by drain/persist commands that need StateIO interface.
func NewStoreWithPassphrase(accountID, region, passphrase string) (*Store, error) {
	store, err := NewStore(accountID, region)
	if err != nil {
		return nil, err
	}

	store.passphrase = passphrase

	return store, nil
}

// SetPassphrase sets the passphrase for encryption/decryption.
// This is primarily for testing.
func (s *Store) SetPassphrase(passphrase string) {
	s.passphrase = passphrase
}

// Exists checks if the state file exists.
func (s *Store) Exists() (bool, error) {
	_, err := os.Stat(s.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to check state file: %w", err)
	}

	return true, nil
}

// IsEncrypted checks if the stored file is encrypted.
func (s *Store) IsEncrypted() (bool, error) {
	data, err := os.ReadFile(s.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to read state file: %w", err)
	}

	return crypt.IsEncrypted(data), nil
}

// Delete removes the state file without reading it.
// This is useful for drop operations that don't need to decrypt the file.
func (s *Store) Delete() error {
	fileMu.Lock()
	defer fileMu.Unlock()

	if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete stash file: %w", err)
	}

	return nil
}

// Drain reads the state from file, optionally deleting the file.
// This implements StateDrainer for file-based storage.
// If service is empty, returns all services; otherwise filters to the specified service.
// If keep is false, the file is deleted after reading.
func (s *Store) Drain(_ context.Context, service staging.Service, keep bool) (*staging.State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	data, err := os.ReadFile(s.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return staging.NewEmptyState(), nil
		}

		return nil, fmt.Errorf("failed to read state file: %w", err)
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
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Initialize maps if nil
	initializeStateMaps(&state)

	// Delete file if keep is false
	if !keep {
		if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove state file: %w", err)
		}
	}

	// Filter by service if specified
	if service != "" {
		return state.ExtractService(service), nil
	}

	return &state, nil
}

// WriteState saves the state to file.
// This implements StateWriter for file-based storage.
// If service is empty, writes all services; otherwise writes only the specified service.
func (s *Store) WriteState(_ context.Context, service staging.Service, state *staging.State) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	// Filter by service if specified
	if service != "" {
		state = state.ExtractService(service)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.stateFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:mnd // owner-only directory permissions
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Check if there are any staged changes
	if state.IsEmpty() {
		// Remove file if no staged changes
		if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty state file: %w", err)
		}

		return nil
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Encrypt if passphrase is provided
	if s.passphrase != "" {
		data, err = crypt.Encrypt(data, s.passphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt state: %w", err)
		}
	}

	if err := os.WriteFile(s.stateFilePath, data, 0o600); err != nil { //nolint:mnd // owner-only file permissions
		return fmt.Errorf("failed to write state file: %w", err)
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

// Compile-time check that Store implements FileStore.
var _ store.FileStore = (*Store)(nil)
