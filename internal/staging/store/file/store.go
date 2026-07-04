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
	stashFileName = "stash.json"
	stateDirName  = ".suve"
)

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

// newStore creates a new file Store using the given file name under
// ~/.suve/{accountID}/{region}/ to isolate state per AWS account and region.
func newStore(accountID, region, fileName string) (*Store, error) {
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, stateDirName, accountID, region)

	return &Store{
		stateFilePath: filepath.Join(stateDir, fileName),
	}, nil
}

// NewStore creates a new file Store with the default state file path.
// The state file is stored under ~/.suve/{accountID}/{region}/stage.json
// to isolate staging state per AWS account and region.
// This is the working staging area used by stage add/edit/delete/status/diff/apply/reset.
func NewStore(accountID, region string) (*Store, error) {
	return newStore(accountID, region, stateFileName)
}

// NewStoreWithPath creates a new file Store with a custom state file path.
// This is primarily for testing.
func NewStoreWithPath(path string) *Store {
	return &Store{
		stateFilePath: path,
	}
}

// NewStoreWithPassphrase creates a new file Store with a passphrase for encryption.
func NewStoreWithPassphrase(accountID, region, passphrase string) (*Store, error) {
	s, err := NewStore(accountID, region)
	if err != nil {
		return nil, err
	}

	s.passphrase = passphrase

	return s, nil
}

// NewStashStore creates a new file Store backed by the stash file.
// The stash file is stored under ~/.suve/{accountID}/{region}/stash.json
// and is used by stage stash push/pop/show/drop.
func NewStashStore(accountID, region string) (*Store, error) {
	return newStore(accountID, region, stashFileName)
}

// NewStashStoreWithPassphrase creates a new stash Store with a passphrase for encryption.
func NewStashStoreWithPassphrase(accountID, region, passphrase string) (*Store, error) {
	s, err := NewStashStore(accountID, region)
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

// readStateLocked reads and decrypts the state from the file.
// The caller must hold fileMu.
// If the file does not exist, an empty state is returned.
func (s *Store) readStateLocked() (*staging.State, error) {
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

	return &state, nil
}

// writeStateLocked writes the state to the file.
// The caller must hold fileMu.
// If the state is empty, the file is removed instead.
func (s *Store) writeStateLocked(state *staging.State) error {
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

// Drain reads the state from file, optionally deleting the file.
// This implements StateDrainer for file-based storage.
// If service is empty, returns all services; otherwise filters to the specified service.
// If keep is false, the file is deleted after reading.
func (s *Store) Drain(_ context.Context, service staging.Service, keep bool) (*staging.State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return nil, err
	}

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

	return state, nil
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

	return s.writeStateLocked(state)
}

// GetEntry retrieves a staged entry.
func (s *Store) GetEntry(_ context.Context, service staging.Service, name string) (*staging.Entry, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return nil, err
	}

	if entry, ok := state.Entries[service][name]; ok {
		return &entry, nil
	}

	return nil, staging.ErrNotStaged
}

// GetTag retrieves staged tag changes.
func (s *Store) GetTag(_ context.Context, service staging.Service, name string) (*staging.TagEntry, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return nil, err
	}

	if tag, ok := state.Tags[service][name]; ok {
		return &tag, nil
	}

	return nil, staging.ErrNotStaged
}

// ListEntries returns all staged entries for a service.
// If service is empty, returns entries for all services.
// Empty service maps are omitted from the result.
func (s *Store) ListEntries(_ context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return nil, err
	}

	result := make(map[staging.Service]map[string]staging.Entry)

	for _, svc := range servicesFor(service) {
		if len(state.Entries[svc]) > 0 {
			result[svc] = state.Entries[svc]
		}
	}

	return result, nil
}

// ListTags returns all staged tag changes for a service.
// If service is empty, returns tags for all services.
// Empty service maps are omitted from the result.
func (s *Store) ListTags(_ context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return nil, err
	}

	result := make(map[staging.Service]map[string]staging.TagEntry)

	for _, svc := range servicesFor(service) {
		if len(state.Tags[svc]) > 0 {
			result[svc] = state.Tags[svc]
		}
	}

	return result, nil
}

// StageEntry adds or updates a staged entry.
func (s *Store) StageEntry(_ context.Context, service staging.Service, name string, entry staging.Entry) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}

	state.Entries[service][name] = entry

	return s.writeStateLocked(state)
}

// StageTag adds or updates staged tag changes.
func (s *Store) StageTag(_ context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}

	state.Tags[service][name] = tagEntry

	return s.writeStateLocked(state)
}

// UnstageEntry removes a staged entry.
func (s *Store) UnstageEntry(_ context.Context, service staging.Service, name string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}

	if _, ok := state.Entries[service][name]; !ok {
		return staging.ErrNotStaged
	}

	delete(state.Entries[service], name)

	return s.writeStateLocked(state)
}

// UnstageTag removes staged tag changes.
func (s *Store) UnstageTag(_ context.Context, service staging.Service, name string) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}

	if _, ok := state.Tags[service][name]; !ok {
		return staging.ErrNotStaged
	}

	delete(state.Tags[service], name)

	return s.writeStateLocked(state)
}

// UnstageAll removes all staged changes for a service.
// If service is empty, all services are cleared.
func (s *Store) UnstageAll(_ context.Context, service staging.Service) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return err
	}

	state.RemoveService(service)

	return s.writeStateLocked(state)
}

// servicesFor returns the services to operate on for the given service filter.
// An empty filter expands to all services.
func servicesFor(service staging.Service) []staging.Service {
	if service == "" {
		return []staging.Service{staging.ServiceParam, staging.ServiceSecret}
	}

	return []staging.Service{service}
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

// Delete removes the state file without reading its contents.
// This is useful for dropping stash when decryption is not needed.
func (s *Store) Delete() error {
	fileMu.Lock()
	defer fileMu.Unlock()

	if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}

	return nil
}

// Compile-time checks that Store implements the storage interfaces.
var (
	_ store.FileStore         = (*Store)(nil)
	_ store.ReadWriteOperator = (*Store)(nil)
)
