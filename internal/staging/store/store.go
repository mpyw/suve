// Package store provides storage interfaces and implementations for staging.
package store

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
)

// ReadOperator provides read-only access to individual staging entries. Items
// are addressed by staging.EntryKey (name + namespace); the namespace is the App
// Configuration label axis (empty for every other provider), and it is part of
// the key for both entries and tags so a namespaced setting is never resolved
// under the wrong namespace.
type ReadOperator interface {
	// GetEntry retrieves the staged entry identified by key.
	GetEntry(ctx context.Context, service staging.Service, key staging.EntryKey) (*staging.Entry, error)
	// GetTag retrieves the staged tag changes identified by key.
	GetTag(ctx context.Context, service staging.Service, key staging.EntryKey) (*staging.TagEntry, error)
	// ListEntries returns all staged entries for a service, keyed by EntryKey.
	ListEntries(ctx context.Context, service staging.Service) (map[staging.Service]map[staging.EntryKey]staging.Entry, error)
	// ListTags returns all staged tag changes for a service, keyed by EntryKey.
	ListTags(ctx context.Context, service staging.Service) (map[staging.Service]map[staging.EntryKey]staging.TagEntry, error)
}

// WriteOperator provides write access to individual staging entries.
type WriteOperator interface {
	// StageEntry adds or updates the staged entry identified by key.
	StageEntry(ctx context.Context, service staging.Service, key staging.EntryKey, entry staging.Entry) error
	// StageTag adds or updates the staged tag changes identified by key.
	StageTag(ctx context.Context, service staging.Service, key staging.EntryKey, tagEntry staging.TagEntry) error
	// UnstageEntry removes the staged entry identified by key.
	UnstageEntry(ctx context.Context, service staging.Service, key staging.EntryKey) error
	// UnstageTag removes the staged tag changes identified by key.
	UnstageTag(ctx context.Context, service staging.Service, key staging.EntryKey) error
	// UnstageAll removes all staged changes for a service.
	UnstageAll(ctx context.Context, service staging.Service) error
}

// ReadWriteOperator combines read and write access to staging entries.
type ReadWriteOperator interface {
	ReadOperator
	WriteOperator
}

// Drainer provides bulk read access to staging state (for drain command).
type Drainer interface {
	// Drain retrieves the entire state from storage.
	// If service is empty, returns all services; otherwise filters to the specified service.
	// If keep is false, the source storage is cleared after reading.
	Drain(ctx context.Context, service staging.Service, keep bool) (*staging.State, error)
}

// Writer provides bulk write access to staging state.
type Writer interface {
	// WriteState writes the entire state to storage.
	// If service is empty, writes all services; otherwise writes only the specified service.
	WriteState(ctx context.Context, service staging.Service, state *staging.State) error
}

// Updater performs an atomic read-modify-write of the whole staging state under
// a single lock hold: it reads the current state fresh, hands it to fn to mutate
// in place, then writes it back — without releasing the lock in between. This
// closes the read-then-write race a separate Drain + WriteState leaves open,
// where a concurrent StageEntry landing between the two is clobbered by the
// stale snapshot. fn must confine itself to mutating the passed state and must
// not call back into the store (the lock is not reentrant).
type Updater interface {
	Update(ctx context.Context, service staging.Service, fn func(*staging.State) error) error
}

// FileStore combines drain and write operations for file storage.
type FileStore interface {
	Drainer
	Writer
}

// WorkingStore is the working-area surface the export and import use cases rely
// on: a bulk Drain for the export snapshot, per-key writes to clear exactly the
// exported keys (each re-read under its own lock), and the atomic Update
// read-modify-write used to reconcile an import without clobbering a concurrent
// stage.
type WorkingStore interface {
	Drainer
	WriteOperator
	Updater
}
