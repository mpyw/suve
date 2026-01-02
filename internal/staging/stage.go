// Package staging provides staging functionality for AWS parameter and secret changes.
package staging

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// Operation represents the type of staged change.
type Operation string

const (
	// OperationCreate represents a create operation (new item).
	OperationCreate Operation = "create"
	// OperationUpdate represents an update operation (existing item).
	OperationUpdate Operation = "update"
	// OperationDelete represents a delete operation.
	OperationDelete Operation = "delete"
)

// Entry represents a single staged change.
type Entry struct {
	Operation   Operation         `json:"operation"`
	Value       string            `json:"value,omitempty"`
	Description *string           `json:"description,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	UntagKeys   []string          `json:"untag_keys,omitempty"`
	StagedAt    time.Time         `json:"staged_at"`
	// DeleteOptions holds SM-specific delete options.
	// Only used when Operation is OperationDelete and service is SM.
	DeleteOptions *DeleteOptions `json:"delete_options,omitempty"`
}

// DeleteOptions holds options for SM delete operations.
type DeleteOptions struct {
	// Force enables immediate permanent deletion without recovery window.
	Force bool `json:"force,omitempty"`
	// RecoveryWindow is the number of days before permanent deletion (7-30).
	// Only used when Force is false. 0 means default (30 days).
	RecoveryWindow int `json:"recovery_window,omitempty"`
}

// State represents the entire staging state.
type State struct {
	Version int              `json:"version"`
	Param   map[string]Entry `json:"param,omitempty"`
	Secret  map[string]Entry `json:"secret,omitempty"`
}

// Service represents which AWS service the staged change belongs to.
type Service string

const (
	// ServiceParam represents AWS Systems Manager Parameter Store.
	ServiceParam Service = "param"
	// ServiceSecret represents AWS Secrets Manager.
	ServiceSecret Service = "secret"
)

const (
	stateVersion  = 1
	stateFileName = "stage.json"
	stateDirName  = ".suve"
)

var (
	// ErrNotStaged is returned when a parameter/secret is not staged.
	ErrNotStaged = errors.New("not staged")
)

// fileMu protects concurrent access to the state file within a process.
var fileMu sync.Mutex

// Store manages the staging state.
type Store struct {
	stateFilePath string
	lockFilePath  string
}

// NewStore creates a new Store with the default state file path.
func NewStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, stateDirName)
	return &Store{
		stateFilePath: filepath.Join(stateDir, stateFileName),
		lockFilePath:  filepath.Join(stateDir, stateFileName+".lock"),
	}, nil
}

// NewStoreWithPath creates a new Store with a custom state file path.
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
func (s *Store) Load() (*State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	return s.loadLocked()
}

func (s *Store) loadLocked() (*State, error) {
	data, err := os.ReadFile(s.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				Version: stateVersion,
				Param:   make(map[string]Entry),
				Secret:  make(map[string]Entry),
			}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Initialize maps if nil
	if state.Param == nil {
		state.Param = make(map[string]Entry)
	}
	if state.Secret == nil {
		state.Secret = make(map[string]Entry)
	}

	return &state, nil
}

// Save saves the staging state to disk.
func (s *Store) Save(state *State) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	return s.saveLocked(state)
}

func (s *Store) saveLocked(state *State) error {
	// Ensure directory exists
	dir := filepath.Dir(s.stateFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Clean up empty maps before saving
	if len(state.Param) == 0 && len(state.Secret) == 0 {
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

// Stage adds or updates a staged change.
func (s *Store) Stage(service Service, name string, entry Entry) error {
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
	case ServiceParam:
		state.Param[name] = entry
	case ServiceSecret:
		state.Secret[name] = entry
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// Unstage removes a staged change.
func (s *Store) Unstage(service Service, name string) error {
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
	case ServiceParam:
		if _, ok := state.Param[name]; !ok {
			return ErrNotStaged
		}
		delete(state.Param, name)
	case ServiceSecret:
		if _, ok := state.Secret[name]; !ok {
			return ErrNotStaged
		}
		delete(state.Secret, name)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageAll removes all staged changes for a service.
// If service is empty, removes all staged changes.
func (s *Store) UnstageAll(service Service) error {
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
	case ServiceParam:
		state.Param = make(map[string]Entry)
	case ServiceSecret:
		state.Secret = make(map[string]Entry)
	case "":
		state.Param = make(map[string]Entry)
		state.Secret = make(map[string]Entry)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// Get retrieves a staged change.
func (s *Store) Get(service Service, name string) (*Entry, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}

	var entry Entry
	var ok bool

	switch service {
	case ServiceParam:
		entry, ok = state.Param[name]
	case ServiceSecret:
		entry, ok = state.Secret[name]
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	if !ok {
		return nil, ErrNotStaged
	}

	return &entry, nil
}

// List returns all staged changes for a service.
// If service is empty, returns all staged changes.
func (s *Store) List(service Service) (map[Service]map[string]Entry, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}

	result := make(map[Service]map[string]Entry)

	switch service {
	case ServiceParam:
		if len(state.Param) > 0 {
			result[ServiceParam] = state.Param
		}
	case ServiceSecret:
		if len(state.Secret) > 0 {
			result[ServiceSecret] = state.Secret
		}
	case "":
		if len(state.Param) > 0 {
			result[ServiceParam] = state.Param
		}
		if len(state.Secret) > 0 {
			result[ServiceSecret] = state.Secret
		}
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	return result, nil
}

// HasChanges returns true if there are any staged changes.
func (s *Store) HasChanges(service Service) (bool, error) {
	state, err := s.Load()
	if err != nil {
		return false, err
	}

	switch service {
	case ServiceParam:
		return len(state.Param) > 0, nil
	case ServiceSecret:
		return len(state.Secret) > 0, nil
	case "":
		return len(state.Param) > 0 || len(state.Secret) > 0, nil
	default:
		return false, fmt.Errorf("unknown service: %s", service)
	}
}

// Count returns the number of staged changes.
func (s *Store) Count(service Service) (int, error) {
	state, err := s.Load()
	if err != nil {
		return 0, err
	}

	switch service {
	case ServiceParam:
		return len(state.Param), nil
	case ServiceSecret:
		return len(state.Secret), nil
	case "":
		return len(state.Param) + len(state.Secret), nil
	default:
		return 0, fmt.Errorf("unknown service: %s", service)
	}
}
