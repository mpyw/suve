// Package file provides file-based staging storage.
// This package implements StateIO interface for drain/persist commands.
//
// Working (staging-area) state is scope-keyed and split per service:
//
//	~/.suve/staging/{scope.Key()}/param.json    (working, param service)
//	~/.suve/staging/{scope.Key()}/secret.json   (working, secret service)
//	~/.suve/staging/{scope.Key()}/stash.json    (stash, single file, all services)
//
// Working files are encrypted with the keychain-resolved data key (raw-key v2);
// the stash file is encrypted with a passphrase (v1). Plaintext/legacy files
// remain readable. A path-based single-file mode is retained for testing and
// for the stash file.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
	"github.com/mpyw/suve/internal/staging/store/file/internal/keyprovider"
)

const (
	stashFileName = "stash.json"
	baseDirName   = ".suve"
	stagingDir    = "staging"
)

// fileMu protects concurrent access to the state files within a process.
//
//nolint:gochecknoglobals // process-wide mutex for file access synchronization
var fileMu sync.Mutex

// Hooks for testing - these allow tests to inject errors.
//
//nolint:gochecknoglobals // test hook for dependency injection
var userHomeDirFunc = os.UserHomeDir

// resolveKeyFunc resolves the working-store data key. Overridable for testing.
//
//nolint:gochecknoglobals // test hook for dependency injection
var resolveKeyFunc = keyprovider.Resolve

// plaintextWarnOnce ensures the plaintext fallback warning is emitted only once
// per process.
//
//nolint:gochecknoglobals // process-wide one-time warning guard.
var plaintextWarnOnce sync.Once

// Store manages the staging state using the filesystem.
// It implements StateIO interface for drain/persist operations.
//
// It operates in one of two modes:
//   - Split (working) mode: stateDir is set and stateFilePath is empty. State
//     is split into param.json / secret.json under stateDir; service=="" ops
//     iterate the scope's supported services.
//   - Single-file mode: stateFilePath is set. The whole state lives in one file
//     (used for the stash file and for path-based tests).
type Store struct {
	// stateDir is the scope directory for split (working) mode.
	stateDir string
	// stateFilePath is the single file path for single-file mode.
	stateFilePath string
	// scope drives supported-service iteration for service=="" operations.
	scope provider.Scope

	passphrase string
	// key, when non-nil, is a 32-byte AES-256 key used for raw-key (v2)
	// encryption of the working store. It takes precedence over passphrase.
	key []byte
}

// scopeDir returns the scope directory ~/.suve/staging/{scope.Key()}.
func scopeDir(scope provider.Scope) (string, error) {
	homeDir, err := userHomeDirFunc()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, baseDirName, stagingDir, scope.Key()), nil
}

// NewStore creates a new split (working) file Store for the given scope.
// State is stored under ~/.suve/staging/{scope.Key()}/ split into
// param.json and secret.json. No encryption key is configured; use
// NewWorkingStore for the encrypted working store.
func NewStore(scope provider.Scope) (*Store, error) {
	dir, err := scopeDir(scope)
	if err != nil {
		return nil, err
	}

	return &Store{
		stateDir: dir,
		scope:    scope,
	}, nil
}

// NewWorkingStore creates the working staging-area Store (split param.json /
// secret.json under the scope directory) with its encryption key resolved via
// the key provider fallback chain:
// SUVE_STAGING_KEY env var -> OS keychain (get-or-create) -> plaintext.
//
// When falling back to plaintext, a one-time warning is emitted to stderr.
// This is the constructor to use for all working-area operations
// (stage add/edit/delete/status/diff/apply/reset and the working side of
// stash push/pop). The stash file (stash.json) keeps its passphrase flow.
func NewWorkingStore(scope provider.Scope) (*Store, error) {
	s, err := NewStore(scope)
	if err != nil {
		return nil, err
	}

	key, plaintext, err := resolveKeyFunc()
	if err != nil {
		// A hard keychain failure (as opposed to a genuinely absent keyring
		// backend or a bad SUVE_STAGING_KEY) is fatal ONLY when encrypted state
		// already exists: reading it needs the key, so the real keychain cause
		// must be surfaced instead of letting a nil key later fail with a
		// misleading "wrong passphrase" error. With no encrypted state yet, the
		// tool stays usable via the documented plaintext fallback (e.g. on
		// headless CI without a keyring), warning the user once.
		var kcErr *keyprovider.KeychainUnavailableError
		if errors.As(err, &kcErr) {
			encrypted, encErr := s.IsEncrypted()
			if encErr != nil {
				return nil, fmt.Errorf("failed to check staging state encryption: %w", encErr)
			}

			if encrypted {
				return nil, fmt.Errorf(
					"cannot access the staging encryption key while encrypted state exists: %w", err)
			}

			warnPlaintextOnce(err)

			return s, nil
		}

		return nil, fmt.Errorf("failed to resolve staging encryption key: %w", err)
	}

	if plaintext {
		warnPlaintextOnce(nil)

		return s, nil
	}

	s.key = key

	return s, nil
}

// warnPlaintextOnce emits the unencrypted-storage warning once per process. When
// cause is non-nil (a hard keychain failure that degraded to plaintext), the
// underlying keychain error is included so the operator can diagnose it.
func warnPlaintextOnce(cause error) {
	plaintextWarnOnce.Do(func() {
		msg := "warning: staging state is stored UNENCRYPTED; " +
			"set SUVE_STAGING_KEY or enable an OS keychain to encrypt"
		if cause != nil {
			msg += "\nwarning: " + cause.Error()
		}

		// Direct stderr write: this low-level store package intentionally
		// avoids depending on the cli/output package.
		//nolint:forbidigo // one-time operator warning to stderr.
		fmt.Fprintln(os.Stderr, msg)
	})
}

// NewStoreWithPath creates a new single-file Store with a custom state file path.
// This is primarily for testing.
func NewStoreWithPath(path string) *Store {
	return &Store{
		stateFilePath: path,
		// A default AWS scope keeps service=="" iteration sensible if ever used.
		scope: provider.Scope{Provider: provider.ProviderAWS},
	}
}

// NewStoreWithPassphrase creates a new split (working) file Store for the given
// scope with a passphrase for encryption. Primarily for testing.
func NewStoreWithPassphrase(scope provider.Scope, passphrase string) (*Store, error) {
	s, err := NewStore(scope)
	if err != nil {
		return nil, err
	}

	s.passphrase = passphrase

	return s, nil
}

// NewStashStore creates a new single-file Store backed by the stash file.
// The stash file is stored under ~/.suve/staging/{scope.Key()}/stash.json
// and is used by stage stash push/pop/show/drop. Unlike the working store,
// the stash file is NOT split: it holds the whole state (all services).
func NewStashStore(scope provider.Scope) (*Store, error) {
	dir, err := scopeDir(scope)
	if err != nil {
		return nil, err
	}

	return &Store{
		stateFilePath: filepath.Join(dir, stashFileName),
		scope:         scope,
	}, nil
}

// NewStashStoreWithPassphrase creates a new stash Store with a passphrase for encryption.
func NewStashStoreWithPassphrase(scope provider.Scope, passphrase string) (*Store, error) {
	s, err := NewStashStore(scope)
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

// isSplit reports whether the store operates in split (per-service) mode.
func (s *Store) isSplit() bool {
	return s.stateFilePath == ""
}

// servicePath returns the per-service file path in split mode.
func (s *Store) servicePath(service staging.Service) string {
	return filepath.Join(s.stateDir, string(service)+".json")
}

// pathFor returns the file path backing the given service. In split mode this
// is the per-service file; in single-file mode it is the single state file.
func (s *Store) pathFor(service staging.Service) string {
	if s.isSplit() {
		return s.servicePath(service)
	}

	return s.stateFilePath
}

// servicesFor returns the services to operate on for the given service filter.
// An empty filter expands to the scope's supported services (registry-driven),
// replacing the previous hardcoded {ServiceParam, ServiceSecret} iteration.
func (s *Store) servicesFor(service staging.Service) []staging.Service {
	if service != "" {
		return []staging.Service{service}
	}

	return staging.SupportedServices(s.scope)
}

// readFile reads and decrypts the state from the given file path.
// If the file does not exist, an empty state is returned.
func (s *Store) readFile(path string) (*staging.State, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is internal, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return staging.NewEmptyState(), nil
		}

		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Decrypt if encrypted. Reading an unencrypted (plaintext/legacy) file is
	// always allowed so migration from an older/plaintext state works.
	if crypt.IsEncrypted(data) {
		switch {
		case s.key != nil:
			data, err = crypt.DecryptWithKey(data, s.key)
		case s.passphrase != "":
			data, err = crypt.Decrypt(data, s.passphrase)
		default:
			return nil, crypt.ErrDecryptionFailed
		}

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

// writeFile writes the state to the given file path.
// If the state is empty, the file is removed instead.
func (s *Store) writeFile(path string, state *staging.State) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:mnd // owner-only directory permissions
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Check if there are any staged changes
	if state.IsEmpty() {
		// Remove file if no staged changes
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty state file: %w", err)
		}

		return nil
	}

	// staging.State implements json.Marshaler (MarshalJSON), emitting its
	// EntryKey-keyed maps as arrays of (name, namespace) records — the EntryKey is
	// never marshaled as a raw map key, so errchkjson's static "unsupported map
	// key" warning is a false positive here.
	data, err := json.MarshalIndent(state, "", "  ") //nolint:errchkjson // State has a custom MarshalJSON
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Encrypt: prefer the raw key (v2) when configured, else fall back to the
	// passphrase (v1); otherwise write plaintext.
	switch {
	case s.key != nil:
		data, err = crypt.EncryptWithKey(data, s.key)
		if err != nil {
			return fmt.Errorf("failed to encrypt state: %w", err)
		}
	case s.passphrase != "":
		data, err = crypt.Encrypt(data, s.passphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt state: %w", err)
		}
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// writeFileAtomic writes data to path atomically: it writes to a temp file in
// the same directory, fsyncs it, then renames it over the target. A crash, OOM,
// or power loss mid-write then leaves either the old file or the complete new
// one — never a truncated/corrupt file, which would fail decryption on the next
// read and feed the error-swallowing overwrite paths. The file is owner-only
// (0600), matching os.CreateTemp's default.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}

	tmpName := tmp.Name()

	// Best-effort cleanup of the temp file if we bail out before renaming.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("failed to sync temp state file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp state file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to rename temp state file: %w", err)
	}

	return nil
}

// fileExists reports whether a file exists at path.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to check state file: %w", err)
	}

	return true, nil
}

// isFileEncrypted reports whether the file at path is encrypted.
func isFileEncrypted(path string) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is internal, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to read state file: %w", err)
	}

	return crypt.IsEncrypted(data), nil
}

// Exists checks if any backing state file exists.
func (s *Store) Exists() (bool, error) {
	if !s.isSplit() {
		return fileExists(s.stateFilePath)
	}

	for _, svc := range s.servicesFor("") {
		ok, err := fileExists(s.servicePath(svc))
		if err != nil {
			return false, err
		}

		if ok {
			return true, nil
		}
	}

	return false, nil
}

// IsEncrypted checks if any backing state file is encrypted.
func (s *Store) IsEncrypted() (bool, error) {
	if !s.isSplit() {
		return isFileEncrypted(s.stateFilePath)
	}

	for _, svc := range s.servicesFor("") {
		enc, err := isFileEncrypted(s.servicePath(svc))
		if err != nil {
			return false, err
		}

		if enc {
			return true, nil
		}
	}

	return false, nil
}

// Drain reads the state from file(s), optionally deleting the file(s).
// This implements StateDrainer for file-based storage.
// If service is empty, returns all services; otherwise filters to the specified service.
// If keep is false, the file(s) read is deleted after reading.
func (s *Store) Drain(_ context.Context, service staging.Service, keep bool) (*staging.State, error) {
	defer s.lock()()

	if !s.isSplit() {
		return s.drainSingle(service, keep)
	}

	if service != "" {
		return s.drainServiceFile(service, keep)
	}

	// Read all supported service files and merge into one whole state.
	merged := staging.NewEmptyState()

	for _, svc := range s.servicesFor("") {
		st, err := s.drainServiceFile(svc, keep)
		if err != nil {
			return nil, err
		}

		merged.Merge(st)
	}

	return merged, nil
}

// drainSingle reads the whole state from the single state file.
// Must be called with fileMu held.
func (s *Store) drainSingle(service staging.Service, keep bool) (*staging.State, error) {
	state, err := s.readFile(s.stateFilePath)
	if err != nil {
		return nil, err
	}

	if !keep {
		if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove state file: %w", err)
		}
	}

	if service != "" {
		return state.ExtractService(service), nil
	}

	return state, nil
}

// drainServiceFile reads the state for a single service's file (split mode).
// Must be called with fileMu held.
func (s *Store) drainServiceFile(service staging.Service, keep bool) (*staging.State, error) {
	path := s.servicePath(service)

	state, err := s.readFile(path)
	if err != nil {
		return nil, err
	}

	if !keep {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove state file: %w", err)
		}
	}

	return state.ExtractService(service), nil
}

// WriteState saves the state to file(s).
// This implements StateWriter for file-based storage.
// If service is empty, writes all supported services; otherwise writes only the specified service.
func (s *Store) WriteState(_ context.Context, service staging.Service, state *staging.State) error {
	defer s.lock()()

	if !s.isSplit() {
		if service != "" {
			state = state.ExtractService(service)
		}

		return s.writeFile(s.stateFilePath, state)
	}

	if service != "" {
		return s.writeFile(s.servicePath(service), state.ExtractService(service))
	}

	for _, svc := range s.servicesFor("") {
		if err := s.writeFile(s.servicePath(svc), state.ExtractService(svc)); err != nil {
			return err
		}
	}

	return nil
}

// GetEntry retrieves the staged entry identified by key.
func (s *Store) GetEntry(_ context.Context, service staging.Service, key staging.EntryKey) (*staging.Entry, error) {
	defer s.lock()()

	state, err := s.readFile(s.pathFor(service))
	if err != nil {
		return nil, err
	}

	if entry, ok := state.Entries[service][key]; ok {
		return &entry, nil
	}

	return nil, staging.ErrNotStaged
}

// GetTag retrieves the staged tag changes identified by key.
func (s *Store) GetTag(_ context.Context, service staging.Service, key staging.EntryKey) (*staging.TagEntry, error) {
	defer s.lock()()

	state, err := s.readFile(s.pathFor(service))
	if err != nil {
		return nil, err
	}

	if tag, ok := state.Tags[service][key]; ok {
		return &tag, nil
	}

	return nil, staging.ErrNotStaged
}

// ListEntries returns all staged entries for a service.
// If service is empty, returns entries for all supported services.
// Empty service maps are omitted from the result.
func (s *Store) ListEntries(_ context.Context, service staging.Service) (map[staging.Service]map[staging.EntryKey]staging.Entry, error) {
	defer s.lock()()

	result := make(map[staging.Service]map[staging.EntryKey]staging.Entry)

	for _, svc := range s.servicesFor(service) {
		state, err := s.readFile(s.pathFor(svc))
		if err != nil {
			return nil, err
		}

		if len(state.Entries[svc]) > 0 {
			result[svc] = state.Entries[svc]
		}
	}

	return result, nil
}

// ListTags returns all staged tag changes for a service.
// If service is empty, returns tags for all supported services.
// Empty service maps are omitted from the result.
func (s *Store) ListTags(_ context.Context, service staging.Service) (map[staging.Service]map[staging.EntryKey]staging.TagEntry, error) {
	defer s.lock()()

	result := make(map[staging.Service]map[staging.EntryKey]staging.TagEntry)

	for _, svc := range s.servicesFor(service) {
		state, err := s.readFile(s.pathFor(svc))
		if err != nil {
			return nil, err
		}

		if len(state.Tags[svc]) > 0 {
			result[svc] = state.Tags[svc]
		}
	}

	return result, nil
}

// writeServiceState persists state to the file backing the given service.
// In split mode only that service's slice is written; in single-file mode the
// whole state is written.
func (s *Store) writeServiceState(service staging.Service, state *staging.State) error {
	if s.isSplit() {
		return s.writeFile(s.servicePath(service), state.ExtractService(service))
	}

	return s.writeFile(s.stateFilePath, state)
}

// StageEntry adds or updates the staged entry identified by key. The stored
// Entry's Namespace is aligned to the key so it never drifts. App Configuration
// settings with the same name under different namespaces are distinct entries.
func (s *Store) StageEntry(_ context.Context, service staging.Service, key staging.EntryKey, entry staging.Entry) error {
	defer s.lock()()

	state, err := s.readFile(s.pathFor(service))
	if err != nil {
		return err
	}

	state.Entries[service][key] = entry

	return s.writeServiceState(service, state)
}

// StageTag adds or updates the staged tag changes identified by key.
func (s *Store) StageTag(_ context.Context, service staging.Service, key staging.EntryKey, tagEntry staging.TagEntry) error {
	defer s.lock()()

	state, err := s.readFile(s.pathFor(service))
	if err != nil {
		return err
	}

	state.Tags[service][key] = tagEntry

	return s.writeServiceState(service, state)
}

// UnstageEntry removes the staged entry identified by key.
func (s *Store) UnstageEntry(_ context.Context, service staging.Service, key staging.EntryKey) error {
	defer s.lock()()

	state, err := s.readFile(s.pathFor(service))
	if err != nil {
		return err
	}

	if _, ok := state.Entries[service][key]; !ok {
		return staging.ErrNotStaged
	}

	delete(state.Entries[service], key)

	return s.writeServiceState(service, state)
}

// UnstageTag removes the staged tag changes identified by key.
func (s *Store) UnstageTag(_ context.Context, service staging.Service, key staging.EntryKey) error {
	defer s.lock()()

	state, err := s.readFile(s.pathFor(service))
	if err != nil {
		return err
	}

	if _, ok := state.Tags[service][key]; !ok {
		return staging.ErrNotStaged
	}

	delete(state.Tags[service], key)

	return s.writeServiceState(service, state)
}

// UnstageAll removes all staged changes for a service.
// If service is empty, all supported services are cleared.
func (s *Store) UnstageAll(_ context.Context, service staging.Service) error {
	defer s.lock()()

	if !s.isSplit() {
		state, err := s.readFile(s.stateFilePath)
		if err != nil {
			return err
		}

		state.RemoveService(service)

		return s.writeFile(s.stateFilePath, state)
	}

	for _, svc := range s.servicesFor(service) {
		state, err := s.readFile(s.servicePath(svc))
		if err != nil {
			return err
		}

		state.RemoveService(svc)

		if err := s.writeFile(s.servicePath(svc), state.ExtractService(svc)); err != nil {
			return err
		}
	}

	return nil
}

// initializeStateMaps ensures all nested maps are initialized.
func initializeStateMaps(state *staging.State) {
	if state.Entries == nil {
		state.Entries = make(map[staging.Service]map[staging.EntryKey]staging.Entry)
	}

	if state.Entries[staging.ServiceParam] == nil {
		state.Entries[staging.ServiceParam] = make(map[staging.EntryKey]staging.Entry)
	}

	if state.Entries[staging.ServiceSecret] == nil {
		state.Entries[staging.ServiceSecret] = make(map[staging.EntryKey]staging.Entry)
	}

	if state.Tags == nil {
		state.Tags = make(map[staging.Service]map[staging.EntryKey]staging.TagEntry)
	}

	if state.Tags[staging.ServiceParam] == nil {
		state.Tags[staging.ServiceParam] = make(map[staging.EntryKey]staging.TagEntry)
	}

	if state.Tags[staging.ServiceSecret] == nil {
		state.Tags[staging.ServiceSecret] = make(map[staging.EntryKey]staging.TagEntry)
	}
}

// Delete removes all backing state files without reading their contents.
// This is useful for dropping stash when decryption is not needed.
func (s *Store) Delete() error {
	defer s.lock()()

	if !s.isSplit() {
		if err := os.Remove(s.stateFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove state file: %w", err)
		}

		return nil
	}

	for _, svc := range s.servicesFor("") {
		if err := os.Remove(s.servicePath(svc)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove state file: %w", err)
		}
	}

	return nil
}

// Compile-time checks that Store implements the storage interfaces.
var (
	_ store.FileStore         = (*Store)(nil)
	_ store.ReadWriteOperator = (*Store)(nil)
)
