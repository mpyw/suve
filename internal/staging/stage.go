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

	"github.com/mpyw/suve/internal/maputil"
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

// Entry represents a single staged entity change (create/update/delete).
// Tags are managed separately in TagEntry.
type Entry struct {
	Operation   Operation `json:"operation"`
	Value       *string   `json:"value,omitempty"` // nil for delete, pointer to distinguish from empty string
	Description *string   `json:"description,omitempty"`
	StagedAt    time.Time `json:"staged_at"`
	// BaseModifiedAt records the AWS LastModified time when the value was fetched.
	// Used for conflict detection: if AWS was modified after this time, it's a conflict.
	// Only set for update/delete operations (nil for create since there's no base).
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
	// DeleteOptions holds Secrets Manager-specific delete options.
	// Only used when Operation is OperationDelete and service is Secrets Manager.
	DeleteOptions *DeleteOptions `json:"delete_options,omitempty"`
}

// TagEntry represents staged tag changes for an entity.
// Managed separately from Entry for cleaner separation of concerns.
type TagEntry struct {
	Add    map[string]string   `json:"add,omitempty"`    // Tags to add or update
	Remove maputil.Set[string] `json:"remove,omitempty"` // Tag keys to remove
	// StagedAt records when the tag change was staged.
	StagedAt time.Time `json:"staged_at"`
	// BaseModifiedAt records the AWS LastModified time when tags were fetched.
	// Used for conflict detection.
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
}

// DeleteOptions holds options for Secrets Manager delete operations.
type DeleteOptions struct {
	// Force enables immediate permanent deletion without recovery window.
	Force bool `json:"force,omitempty"`
	// RecoveryWindow is the number of days before permanent deletion (7-30).
	// Only used when Force is false. 0 means default (30 days).
	RecoveryWindow int `json:"recovery_window,omitempty"`
}

// State represents the entire staging state (v2).
// Entries and Tags are managed separately for cleaner separation of concerns.
type State struct {
	Version int                             `json:"version"`
	Entries map[Service]map[string]Entry    `json:"entries,omitempty"`
	Tags    map[Service]map[string]TagEntry `json:"tags,omitempty"`
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
	stateVersion  = 2
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
			return newEmptyState(), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Initialize maps if nil
	initializeStateMaps(&state)

	return &state, nil
}

// newEmptyState creates a new empty state with initialized maps.
func newEmptyState() *State {
	return &State{
		Version: stateVersion,
		Entries: map[Service]map[string]Entry{
			ServiceParam:  make(map[string]Entry),
			ServiceSecret: make(map[string]Entry),
		},
		Tags: map[Service]map[string]TagEntry{
			ServiceParam:  make(map[string]TagEntry),
			ServiceSecret: make(map[string]TagEntry),
		},
	}
}

// initializeStateMaps ensures all nested maps are initialized.
func initializeStateMaps(state *State) {
	if state.Entries == nil {
		state.Entries = make(map[Service]map[string]Entry)
	}
	if state.Entries[ServiceParam] == nil {
		state.Entries[ServiceParam] = make(map[string]Entry)
	}
	if state.Entries[ServiceSecret] == nil {
		state.Entries[ServiceSecret] = make(map[string]Entry)
	}
	if state.Tags == nil {
		state.Tags = make(map[Service]map[string]TagEntry)
	}
	if state.Tags[ServiceParam] == nil {
		state.Tags[ServiceParam] = make(map[string]TagEntry)
	}
	if state.Tags[ServiceSecret] == nil {
		state.Tags[ServiceSecret] = make(map[string]TagEntry)
	}
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

	// Check if there are any staged changes
	hasEntries := len(state.Entries[ServiceParam]) > 0 || len(state.Entries[ServiceSecret]) > 0
	hasTags := len(state.Tags[ServiceParam]) > 0 || len(state.Tags[ServiceSecret]) > 0

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

// StageEntry adds or updates a staged entry change.
func (s *Store) StageEntry(service Service, name string, entry Entry) error {
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
	case ServiceParam, ServiceSecret:
		state.Entries[service][name] = entry
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// StageTag adds or updates staged tag changes.
func (s *Store) StageTag(service Service, name string, tagEntry TagEntry) error {
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
	case ServiceParam, ServiceSecret:
		state.Tags[service][name] = tagEntry
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageEntry removes a staged entry change.
func (s *Store) UnstageEntry(service Service, name string) error {
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
	case ServiceParam, ServiceSecret:
		if _, ok := state.Entries[service][name]; !ok {
			return ErrNotStaged
		}
		delete(state.Entries[service], name)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageTag removes staged tag changes.
func (s *Store) UnstageTag(service Service, name string) error {
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
	case ServiceParam, ServiceSecret:
		if _, ok := state.Tags[service][name]; !ok {
			return ErrNotStaged
		}
		delete(state.Tags[service], name)
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
		state.Entries[ServiceParam] = make(map[string]Entry)
		state.Tags[ServiceParam] = make(map[string]TagEntry)
	case ServiceSecret:
		state.Entries[ServiceSecret] = make(map[string]Entry)
		state.Tags[ServiceSecret] = make(map[string]TagEntry)
	case "":
		state.Entries[ServiceParam] = make(map[string]Entry)
		state.Entries[ServiceSecret] = make(map[string]Entry)
		state.Tags[ServiceParam] = make(map[string]TagEntry)
		state.Tags[ServiceSecret] = make(map[string]TagEntry)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// GetEntry retrieves a staged entry.
func (s *Store) GetEntry(service Service, name string) (*Entry, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}

	switch service {
	case ServiceParam, ServiceSecret:
		entry, ok := state.Entries[service][name]
		if !ok {
			return nil, ErrNotStaged
		}
		return &entry, nil
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}
}

// GetTag retrieves staged tag changes.
func (s *Store) GetTag(service Service, name string) (*TagEntry, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}

	switch service {
	case ServiceParam, ServiceSecret:
		tagEntry, ok := state.Tags[service][name]
		if !ok {
			return nil, ErrNotStaged
		}
		return &tagEntry, nil
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}
}

// ListEntries returns all staged entries for a service.
// If service is empty, returns all staged entries.
func (s *Store) ListEntries(service Service) (map[Service]map[string]Entry, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}

	result := make(map[Service]map[string]Entry)

	switch service {
	case ServiceParam:
		if len(state.Entries[ServiceParam]) > 0 {
			result[ServiceParam] = state.Entries[ServiceParam]
		}
	case ServiceSecret:
		if len(state.Entries[ServiceSecret]) > 0 {
			result[ServiceSecret] = state.Entries[ServiceSecret]
		}
	case "":
		if len(state.Entries[ServiceParam]) > 0 {
			result[ServiceParam] = state.Entries[ServiceParam]
		}
		if len(state.Entries[ServiceSecret]) > 0 {
			result[ServiceSecret] = state.Entries[ServiceSecret]
		}
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	return result, nil
}

// ListTags returns all staged tag changes for a service.
// If service is empty, returns all staged tag changes.
func (s *Store) ListTags(service Service) (map[Service]map[string]TagEntry, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}

	result := make(map[Service]map[string]TagEntry)

	switch service {
	case ServiceParam:
		if len(state.Tags[ServiceParam]) > 0 {
			result[ServiceParam] = state.Tags[ServiceParam]
		}
	case ServiceSecret:
		if len(state.Tags[ServiceSecret]) > 0 {
			result[ServiceSecret] = state.Tags[ServiceSecret]
		}
	case "":
		if len(state.Tags[ServiceParam]) > 0 {
			result[ServiceParam] = state.Tags[ServiceParam]
		}
		if len(state.Tags[ServiceSecret]) > 0 {
			result[ServiceSecret] = state.Tags[ServiceSecret]
		}
	default:
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	return result, nil
}

// HasChanges returns true if there are any staged changes (entries or tags).
func (s *Store) HasChanges(service Service) (bool, error) {
	state, err := s.Load()
	if err != nil {
		return false, err
	}

	switch service {
	case ServiceParam:
		return len(state.Entries[ServiceParam]) > 0 || len(state.Tags[ServiceParam]) > 0, nil
	case ServiceSecret:
		return len(state.Entries[ServiceSecret]) > 0 || len(state.Tags[ServiceSecret]) > 0, nil
	case "":
		return len(state.Entries[ServiceParam]) > 0 || len(state.Entries[ServiceSecret]) > 0 ||
			len(state.Tags[ServiceParam]) > 0 || len(state.Tags[ServiceSecret]) > 0, nil
	default:
		return false, fmt.Errorf("unknown service: %s", service)
	}
}

// Count returns the number of staged entry changes.
// Note: This counts entries only, not tag changes. Use CountAll for total count.
func (s *Store) Count(service Service) (int, error) {
	state, err := s.Load()
	if err != nil {
		return 0, err
	}

	switch service {
	case ServiceParam:
		return len(state.Entries[ServiceParam]), nil
	case ServiceSecret:
		return len(state.Entries[ServiceSecret]), nil
	case "":
		return len(state.Entries[ServiceParam]) + len(state.Entries[ServiceSecret]), nil
	default:
		return 0, fmt.Errorf("unknown service: %s", service)
	}
}
