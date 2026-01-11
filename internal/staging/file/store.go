// Package file provides file-based staging storage.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/crypt"
)

const (
	stateVersion  = 2
	stateFileName = "stage.json"
	stateDirName  = ".suve"
)

// fileMu protects concurrent access to the state file within a process.
var fileMu sync.Mutex

// Store manages the staging state using the filesystem.
type Store struct {
	stateFilePath string
	lockFilePath  string
}

// NewStore creates a new file Store with the default state file path.
// The state file is stored under ~/.suve/{accountID}/{region}/stage.json
// to isolate staging state per AWS account and region.
func NewStore(accountID, region string) (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, stateDirName, accountID, region)
	return &Store{
		stateFilePath: filepath.Join(stateDir, stateFileName),
		lockFilePath:  filepath.Join(stateDir, stateFileName+".lock"),
	}, nil
}

// NewStoreWithPath creates a new file Store with a custom state file path.
// This is primarily for testing.
func NewStoreWithPath(path string) *Store {
	return &Store{
		stateFilePath: path,
		lockFilePath:  path + ".lock",
	}
}

// acquireFileLock acquires an exclusive file lock for cross-process synchronization.
// Returns the flock that must be unlocked to release the lock.
func (s *Store) acquireFileLock() (*flock.Flock, error) {
	// Ensure directory exists
	dir := filepath.Dir(s.lockFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	fileLock := flock.New(s.lockFilePath)

	// Acquire exclusive lock (blocks until lock is available)
	if err := fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("failed to acquire file lock: %w", errors.Join(
			err,
			fileLock.Close(),
		))
	}

	return fileLock, nil
}

// releaseFileLock releases the file lock.
func (s *Store) releaseFileLock(fileLock *flock.Flock) {
	if fileLock != nil {
		_ = fileLock.Unlock()
	}
}

// Load loads the current staging state from disk.
func (s *Store) Load(_ context.Context) (*staging.State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	return s.loadLocked()
}

func (s *Store) loadLocked() (*staging.State, error) {
	data, err := os.ReadFile(s.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return newEmptyState(), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state staging.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Initialize maps if nil
	initializeStateMaps(&state)

	return &state, nil
}

// newEmptyState creates a new empty state with initialized maps.
func newEmptyState() *staging.State {
	return &staging.State{
		Version: stateVersion,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  make(map[string]staging.Entry),
			staging.ServiceSecret: make(map[string]staging.Entry),
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  make(map[string]staging.TagEntry),
			staging.ServiceSecret: make(map[string]staging.TagEntry),
		},
	}
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

// Save saves the staging state to disk.
func (s *Store) Save(state *staging.State) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	return s.saveLocked(state)
}

func (s *Store) saveLocked(state *staging.State) error {
	// Ensure directory exists
	dir := filepath.Dir(s.stateFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Check if there are any staged changes
	hasEntries := len(state.Entries[staging.ServiceParam]) > 0 || len(state.Entries[staging.ServiceSecret]) > 0
	hasTags := len(state.Tags[staging.ServiceParam]) > 0 || len(state.Tags[staging.ServiceSecret]) > 0

	if !hasEntries && !hasTags {
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

	if err := os.WriteFile(s.stateFilePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// SaveWithPassphrase saves the staging state to disk, encrypting if passphrase is non-empty.
func (s *Store) SaveWithPassphrase(state *staging.State, passphrase string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(s.stateFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Check if there are any staged changes
	hasEntries := len(state.Entries[staging.ServiceParam]) > 0 || len(state.Entries[staging.ServiceSecret]) > 0
	hasTags := len(state.Tags[staging.ServiceParam]) > 0 || len(state.Tags[staging.ServiceSecret]) > 0

	if !hasEntries && !hasTags {
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
	if passphrase != "" {
		data, err = crypt.Encrypt(data, passphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt state: %w", err)
		}
	}

	if err := os.WriteFile(s.stateFilePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadWithPassphrase loads the staging state from disk, decrypting if necessary.
// If the file is encrypted and passphrase is empty, returns crypt.ErrDecryptionFailed.
func (s *Store) LoadWithPassphrase(passphrase string) (*staging.State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	data, err := os.ReadFile(s.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return newEmptyState(), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Decrypt if encrypted
	if crypt.IsEncrypted(data) {
		if passphrase == "" {
			return nil, crypt.ErrDecryptionFailed
		}
		data, err = crypt.Decrypt(data, passphrase)
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

	return &state, nil
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

// StageEntry adds or updates a staged entry change.
func (s *Store) StageEntry(_ context.Context, service staging.Service, name string, entry staging.Entry) error {
	// Acquire cross-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return err
	}
	defer s.releaseFileLock(lockFile)

	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case staging.ServiceParam, staging.ServiceSecret:
		state.Entries[service][name] = entry
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// StageTag adds or updates staged tag changes.
func (s *Store) StageTag(_ context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error {
	// Acquire cross-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return err
	}
	defer s.releaseFileLock(lockFile)

	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case staging.ServiceParam, staging.ServiceSecret:
		state.Tags[service][name] = tagEntry
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageEntry removes a staged entry change.
func (s *Store) UnstageEntry(_ context.Context, service staging.Service, name string) error {
	// Acquire cross-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return err
	}
	defer s.releaseFileLock(lockFile)

	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case staging.ServiceParam, staging.ServiceSecret:
		if _, ok := state.Entries[service][name]; !ok {
			return staging.ErrNotStaged
		}
		delete(state.Entries[service], name)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageTag removes staged tag changes.
func (s *Store) UnstageTag(_ context.Context, service staging.Service, name string) error {
	// Acquire cross-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return err
	}
	defer s.releaseFileLock(lockFile)

	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case staging.ServiceParam, staging.ServiceSecret:
		if _, ok := state.Tags[service][name]; !ok {
			return staging.ErrNotStaged
		}
		delete(state.Tags[service], name)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageAll removes all staged changes for a service.
// If service is empty, removes all staged changes.
func (s *Store) UnstageAll(_ context.Context, service staging.Service) error {
	// Acquire cross-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return err
	}
	defer s.releaseFileLock(lockFile)

	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case staging.ServiceParam:
		state.Entries[staging.ServiceParam] = make(map[string]staging.Entry)
		state.Tags[staging.ServiceParam] = make(map[string]staging.TagEntry)
	case staging.ServiceSecret:
		state.Entries[staging.ServiceSecret] = make(map[string]staging.Entry)
		state.Tags[staging.ServiceSecret] = make(map[string]staging.TagEntry)
	case "":
		state.Entries[staging.ServiceParam] = make(map[string]staging.Entry)
		state.Entries[staging.ServiceSecret] = make(map[string]staging.Entry)
		state.Tags[staging.ServiceParam] = make(map[string]staging.TagEntry)
		state.Tags[staging.ServiceSecret] = make(map[string]staging.TagEntry)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// GetEntry retrieves a staged entry.
func (s *Store) GetEntry(ctx context.Context, service staging.Service, name string) (*staging.Entry, error) {
	state, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}

	switch service {
	case staging.ServiceParam, staging.ServiceSecret:
		entry, ok := state.Entries[service][name]
		if !ok {
			return nil, staging.ErrNotStaged
		}
		return &entry, nil
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}
}

// GetTag retrieves staged tag changes.
func (s *Store) GetTag(ctx context.Context, service staging.Service, name string) (*staging.TagEntry, error) {
	state, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}

	switch service {
	case staging.ServiceParam, staging.ServiceSecret:
		tagEntry, ok := state.Tags[service][name]
		if !ok {
			return nil, staging.ErrNotStaged
		}
		return &tagEntry, nil
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}
}

// ListEntries returns all staged entries for a service.
// If service is empty, returns all staged entries.
func (s *Store) ListEntries(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
	state, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[staging.Service]map[string]staging.Entry)

	switch service {
	case staging.ServiceParam:
		if len(state.Entries[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = state.Entries[staging.ServiceParam]
		}
	case staging.ServiceSecret:
		if len(state.Entries[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = state.Entries[staging.ServiceSecret]
		}
	case "":
		if len(state.Entries[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = state.Entries[staging.ServiceParam]
		}
		if len(state.Entries[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = state.Entries[staging.ServiceSecret]
		}
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	return result, nil
}

// ListTags returns all staged tag changes for a service.
// If service is empty, returns all staged tag changes.
func (s *Store) ListTags(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	state, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[staging.Service]map[string]staging.TagEntry)

	switch service {
	case staging.ServiceParam:
		if len(state.Tags[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = state.Tags[staging.ServiceParam]
		}
	case staging.ServiceSecret:
		if len(state.Tags[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = state.Tags[staging.ServiceSecret]
		}
	case "":
		if len(state.Tags[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = state.Tags[staging.ServiceParam]
		}
		if len(state.Tags[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = state.Tags[staging.ServiceSecret]
		}
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	return result, nil
}

// HasChanges returns true if there are any staged changes (entries or tags).
func (s *Store) HasChanges(ctx context.Context, service staging.Service) (bool, error) {
	state, err := s.Load(ctx)
	if err != nil {
		return false, err
	}

	switch service {
	case staging.ServiceParam:
		return len(state.Entries[staging.ServiceParam]) > 0 || len(state.Tags[staging.ServiceParam]) > 0, nil
	case staging.ServiceSecret:
		return len(state.Entries[staging.ServiceSecret]) > 0 || len(state.Tags[staging.ServiceSecret]) > 0, nil
	case "":
		return len(state.Entries[staging.ServiceParam]) > 0 || len(state.Entries[staging.ServiceSecret]) > 0 ||
			len(state.Tags[staging.ServiceParam]) > 0 || len(state.Tags[staging.ServiceSecret]) > 0, nil
	default:
		return false, fmt.Errorf("unknown service: %s", service)
	}
}

// Count returns the number of staged entry changes.
// Note: This counts entries only, not tag changes.
func (s *Store) Count(ctx context.Context, service staging.Service) (int, error) {
	state, err := s.Load(ctx)
	if err != nil {
		return 0, err
	}

	switch service {
	case staging.ServiceParam:
		return len(state.Entries[staging.ServiceParam]), nil
	case staging.ServiceSecret:
		return len(state.Entries[staging.ServiceSecret]), nil
	case "":
		return len(state.Entries[staging.ServiceParam]) + len(state.Entries[staging.ServiceSecret]), nil
	default:
		return 0, fmt.Errorf("unknown service: %s", service)
	}
}
