// Package file provides file-based staging storage.
// This package implements StateIO interface for drain/persist commands.
//
// V3 storage format uses separate files per service:
//   - ~/.suve/{accountID}/{region}/param.json
//   - ~/.suve/{accountID}/{region}/secret.json
//
// V3 file format (simplified, service-specific):
//
//	{
//	  "version": 3,
//	  "service": "param",
//	  "entries": {"/app/config": {...}},
//	  "tags": {"/app/config": {...}}
//	}
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

// fileState represents the V3 file format for a single service.
// This is a simplified format without service nesting since each file is service-specific.
type fileState struct {
	Version int                         `json:"version"`
	Service staging.Service             `json:"service"`
	Entries map[string]staging.Entry    `json:"entries,omitempty"`
	Tags    map[string]staging.TagEntry `json:"tags,omitempty"`
}

const (
	stateDirName = ".suve"
)

// AllServices is the list of all services for iteration.
//
//nolint:gochecknoglobals // package-wide constant for service enumeration
var AllServices = []staging.Service{staging.ServiceParam, staging.ServiceSecret}

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

// Store manages the staging state for a specific service using the filesystem.
// Each service has its own file (param.json, secret.json).
// It implements StateIO interface for drain/persist operations.
type Store struct {
	stateFilePath string
	service       staging.Service
	passphrase    string
}

// NewStore creates a new file Store for a specific service.
// The state file is stored under ~/.suve/{accountID}/{region}/{service}.json
// to isolate staging state per AWS account, region, and service.
func NewStore(accountID, region string, service staging.Service) (*Store, error) {
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, stateDirName, accountID, region)
	fileName := string(service) + ".json"

	return &Store{
		stateFilePath: filepath.Join(stateDir, fileName),
		service:       service,
	}, nil
}

// NewStoreWithPath creates a new file Store with a custom state file path.
// This is primarily for testing.
func NewStoreWithPath(path string, service staging.Service) *Store {
	return &Store{
		stateFilePath: path,
		service:       service,
	}
}

// NewStoreWithPassphrase creates a new file Store with a passphrase for encryption.
// This is used by drain/persist commands that need StateIO interface.
func NewStoreWithPassphrase(accountID, region string, service staging.Service, passphrase string) (*Store, error) {
	store, err := NewStore(accountID, region, service)
	if err != nil {
		return nil, err
	}

	store.passphrase = passphrase

	return store, nil
}

// NewStoresForAllServices creates stores for all services.
// This is used by global operations that need to operate on all services.
func NewStoresForAllServices(accountID, region string) (map[staging.Service]*Store, error) {
	stores := make(map[staging.Service]*Store)

	for _, svc := range AllServices {
		store, err := NewStore(accountID, region, svc)
		if err != nil {
			return nil, err
		}

		stores[svc] = store
	}

	return stores, nil
}

// NewStoresWithPassphrase creates stores for all services with a passphrase.
func NewStoresWithPassphrase(accountID, region, passphrase string) (map[staging.Service]*Store, error) {
	stores := make(map[staging.Service]*Store)

	for _, svc := range AllServices {
		store, err := NewStoreWithPassphrase(accountID, region, svc, passphrase)
		if err != nil {
			return nil, err
		}

		stores[svc] = store
	}

	return stores, nil
}

// Service returns the service this store is for.
func (s *Store) Service() staging.Service {
	return s.service
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

// Drain reads the state from file, optionally deleting the file.
// This implements StateDrainer for file-based storage.
// In V3, each store is service-specific. The service parameter is kept for interface compatibility:
//   - If service is empty or matches the store's service: operates on this store's file
//   - If service doesn't match: returns empty state (no-op)
//
// If keep is false, the file is deleted after reading.
func (s *Store) Drain(_ context.Context, service staging.Service, keep bool) (*staging.State, error) {
	// Service mismatch check: if a different service is requested, return empty
	if service != "" && service != s.service {
		return staging.NewEmptyState(), nil
	}

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

	// Parse V3 file format
	var fs fileState
	if err := json.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Convert to State
	state := staging.NewEmptyState()
	state.Version = fs.Version

	if fs.Entries != nil {
		state.Entries[s.service] = fs.Entries
	}

	if fs.Tags != nil {
		state.Tags[s.service] = fs.Tags
	}

	// Delete file if keep is false
	if !keep {
		if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove state file: %w", err)
		}
	}

	return state, nil
}

// WriteState saves the state to file.
// This implements StateWriter for file-based storage.
// In V3, each store is service-specific. The service parameter is kept for interface compatibility:
//   - If service is empty or matches the store's service: operates on this store's file
//   - If service doesn't match: no-op (returns nil)
func (s *Store) WriteState(_ context.Context, service staging.Service, state *staging.State) error {
	// Service mismatch check: if a different service is requested, no-op
	if service != "" && service != s.service {
		return nil
	}

	fileMu.Lock()
	defer fileMu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(s.stateFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:mnd // owner-only directory permissions
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Extract only this store's service data
	entries := state.Entries[s.service]
	tags := state.Tags[s.service]

	// Check if there are any staged changes
	if len(entries) == 0 && len(tags) == 0 {
		// Remove file if no staged changes
		if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty state file: %w", err)
		}

		return nil
	}

	// Convert to V3 file format
	fs := fileState{
		Version: state.Version,
		Service: s.service,
		Entries: entries,
		Tags:    tags,
	}

	data, err := json.MarshalIndent(fs, "", "  ")
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

// DrainAll reads and merges state from multiple stores.
// This is a convenience function for global operations.
func DrainAll(ctx context.Context, stores map[staging.Service]*Store, keep bool) (*staging.State, error) {
	result := staging.NewEmptyState()

	for _, svc := range AllServices {
		store, ok := stores[svc]
		if !ok {
			continue
		}

		state, err := store.Drain(ctx, "", keep)
		if err != nil {
			return nil, fmt.Errorf("failed to drain %s: %w", svc, err)
		}

		result.Merge(state)
	}

	return result, nil
}

// WriteAll writes state to multiple stores.
// This is a convenience function for global operations.
func WriteAll(ctx context.Context, stores map[staging.Service]*Store, state *staging.State) error {
	for _, svc := range AllServices {
		store, ok := stores[svc]
		if !ok {
			continue
		}

		if err := store.WriteState(ctx, svc, state); err != nil {
			return fmt.Errorf("failed to write %s: %w", svc, err)
		}
	}

	return nil
}

// DeleteAll deletes all service files.
// This is a convenience function for global drop operations.
func DeleteAll(stores map[staging.Service]*Store) error {
	for _, svc := range AllServices {
		store, ok := stores[svc]
		if !ok {
			continue
		}

		if err := store.Delete(); err != nil {
			return fmt.Errorf("failed to delete %s: %w", svc, err)
		}
	}

	return nil
}

// AnyExists checks if any of the stores' files exist.
func AnyExists(stores map[staging.Service]*Store) (bool, error) {
	for _, store := range stores {
		exists, err := store.Exists()
		if err != nil {
			return false, err
		}

		if exists {
			return true, nil
		}
	}

	return false, nil
}

// AnyEncrypted checks if any of the stores' files are encrypted.
func AnyEncrypted(stores map[staging.Service]*Store) (bool, error) {
	for _, store := range stores {
		exists, err := store.Exists()
		if err != nil {
			return false, err
		}

		if !exists {
			continue
		}

		encrypted, err := store.IsEncrypted()
		if err != nil {
			return false, err
		}

		if encrypted {
			return true, nil
		}
	}

	return false, nil
}

// CompositeStore wraps multiple service-specific stores and implements FileStore interface.
// This allows usecases to work with V3 multi-file storage without changes.
type CompositeStore struct {
	stores map[staging.Service]*Store
}

// NewCompositeStore creates a composite store wrapping multiple service stores.
func NewCompositeStore(stores map[staging.Service]*Store) *CompositeStore {
	return &CompositeStore{stores: stores}
}

// Drain reads and merges state from all underlying stores.
// If service is specified, only drains from that service's store.
func (c *CompositeStore) Drain(ctx context.Context, service staging.Service, keep bool) (*staging.State, error) {
	if service != "" {
		// Service-specific drain
		store, ok := c.stores[service]
		if !ok {
			return staging.NewEmptyState(), nil
		}

		return store.Drain(ctx, service, keep)
	}

	// Global drain - merge all stores
	return DrainAll(ctx, c.stores, keep)
}

// WriteState writes state to appropriate store(s).
// If service is specified, writes only to that service's store.
func (c *CompositeStore) WriteState(ctx context.Context, service staging.Service, state *staging.State) error {
	if service != "" {
		// Service-specific write
		store, ok := c.stores[service]
		if !ok {
			return nil
		}

		return store.WriteState(ctx, service, state)
	}

	// Global write - write to all stores
	return WriteAll(ctx, c.stores, state)
}

// Compile-time check that Store implements FileStore.
var _ store.FileStore = (*Store)(nil)

// Compile-time check that CompositeStore implements FileStore.
var _ store.FileStore = (*CompositeStore)(nil)
