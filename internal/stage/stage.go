// Package stage provides staging functionality for AWS parameter and secret changes.
package stage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Operation represents the type of staged change.
type Operation string

const (
	// OperationSet represents a set/update operation.
	OperationSet Operation = "set"
	// OperationDelete represents a delete operation.
	OperationDelete Operation = "delete"
)

// Entry represents a single staged change.
type Entry struct {
	Operation Operation `json:"operation"`
	Value     string    `json:"value,omitempty"`
	StagedAt  time.Time `json:"staged_at"`
}

// State represents the entire staging state.
type State struct {
	Version int              `json:"version"`
	SSM     map[string]Entry `json:"ssm,omitempty"`
	SM      map[string]Entry `json:"sm,omitempty"`
}

// Service represents which AWS service the staged change belongs to.
type Service string

const (
	// ServiceSSM represents AWS Systems Manager Parameter Store.
	ServiceSSM Service = "ssm"
	// ServiceSM represents AWS Secrets Manager.
	ServiceSM Service = "sm"
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

// fileMu protects concurrent access to the state file.
var fileMu sync.Mutex

// Store manages the staging state.
type Store struct {
	stateFilePath string
}

// NewStore creates a new Store with the default state file path.
func NewStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Store{
		stateFilePath: filepath.Join(homeDir, stateDirName, stateFileName),
	}, nil
}

// NewStoreWithPath creates a new Store with a custom state file path.
// This is primarily for testing.
func NewStoreWithPath(path string) *Store {
	return &Store{stateFilePath: path}
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
				SSM:     make(map[string]Entry),
				SM:      make(map[string]Entry),
			}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Initialize maps if nil
	if state.SSM == nil {
		state.SSM = make(map[string]Entry)
	}
	if state.SM == nil {
		state.SM = make(map[string]Entry)
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
	if len(state.SSM) == 0 && len(state.SM) == 0 {
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
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case ServiceSSM:
		state.SSM[name] = entry
	case ServiceSM:
		state.SM[name] = entry
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// Unstage removes a staged change.
func (s *Store) Unstage(service Service, name string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case ServiceSSM:
		if _, ok := state.SSM[name]; !ok {
			return ErrNotStaged
		}
		delete(state.SSM, name)
	case ServiceSM:
		if _, ok := state.SM[name]; !ok {
			return ErrNotStaged
		}
		delete(state.SM, name)
	default:
		return fmt.Errorf("unknown service: %s", service)
	}

	return s.saveLocked(state)
}

// UnstageAll removes all staged changes for a service.
// If service is empty, removes all staged changes.
func (s *Store) UnstageAll(service Service) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}

	switch service {
	case ServiceSSM:
		state.SSM = make(map[string]Entry)
	case ServiceSM:
		state.SM = make(map[string]Entry)
	case "":
		state.SSM = make(map[string]Entry)
		state.SM = make(map[string]Entry)
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
	case ServiceSSM:
		entry, ok = state.SSM[name]
	case ServiceSM:
		entry, ok = state.SM[name]
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
	case ServiceSSM:
		if len(state.SSM) > 0 {
			result[ServiceSSM] = state.SSM
		}
	case ServiceSM:
		if len(state.SM) > 0 {
			result[ServiceSM] = state.SM
		}
	case "":
		if len(state.SSM) > 0 {
			result[ServiceSSM] = state.SSM
		}
		if len(state.SM) > 0 {
			result[ServiceSM] = state.SM
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
	case ServiceSSM:
		return len(state.SSM) > 0, nil
	case ServiceSM:
		return len(state.SM) > 0, nil
	case "":
		return len(state.SSM) > 0 || len(state.SM) > 0, nil
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
	case ServiceSSM:
		return len(state.SSM), nil
	case ServiceSM:
		return len(state.SM), nil
	case "":
		return len(state.SSM) + len(state.SM), nil
	default:
		return 0, fmt.Errorf("unknown service: %s", service)
	}
}
