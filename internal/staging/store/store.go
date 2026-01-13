// Package store provides storage interfaces and implementations for staging.
package store

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
)

// Hint values for context-aware shutdown messages.
const (
	HintApply   = "apply"   // Unstage triggered by apply (changes were applied to AWS)
	HintReset   = "reset"   // Unstage triggered by reset (changes were discarded)
	HintPersist = "persist" // Unstage triggered by persist (state saved to file)
)

// ReadOperator provides read-only access to individual staging entries.
type ReadOperator interface {
	// GetEntry retrieves a staged entry.
	GetEntry(ctx context.Context, service staging.Service, name string) (*staging.Entry, error)
	// GetTag retrieves staged tag changes.
	GetTag(ctx context.Context, service staging.Service, name string) (*staging.TagEntry, error)
	// ListEntries returns all staged entries for a service.
	ListEntries(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error)
	// ListTags returns all staged tag changes for a service.
	ListTags(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error)
}

// WriteOperator provides write access to individual staging entries.
type WriteOperator interface {
	// StageEntry adds or updates a staged entry.
	StageEntry(ctx context.Context, service staging.Service, name string, entry staging.Entry) error
	// StageTag adds or updates staged tag changes.
	StageTag(ctx context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error
	// UnstageEntry removes a staged entry.
	UnstageEntry(ctx context.Context, service staging.Service, name string) error
	// UnstageTag removes staged tag changes.
	UnstageTag(ctx context.Context, service staging.Service, name string) error
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

// AgentStore provides full access to agent storage including drain/write operations.
type AgentStore interface {
	ReadWriteOperator
	Drainer
	Writer
}

// HintedUnstager provides unstage operations with hints for context-aware shutdown messages.
// This interface is optional and only implemented by agent stores.
type HintedUnstager interface {
	// UnstageEntryWithHint removes a staged entry with an operation hint.
	UnstageEntryWithHint(ctx context.Context, service staging.Service, name string, hint string) error
	// UnstageTagWithHint removes staged tag changes with an operation hint.
	UnstageTagWithHint(ctx context.Context, service staging.Service, name string, hint string) error
	// UnstageAllWithHint removes all staged changes with an operation hint.
	UnstageAllWithHint(ctx context.Context, service staging.Service, hint string) error
}
