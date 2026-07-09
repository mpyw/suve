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

// FileStore combines drain and write operations for file storage.
type FileStore interface {
	Drainer
	Writer
}
