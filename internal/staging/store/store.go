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

// =============================================================================
// New interfaces (no service parameter - scope determined at creation time)
// =============================================================================

// Drainer provides bulk read access to staging state.
// The scope (single service or all services) is determined at creation time.
type Drainer interface {
	// Drain retrieves state from storage.
	// If keep is false, the source storage is cleared after reading.
	Drain(ctx context.Context, keep bool) (*staging.State, error)
}

// Writer provides bulk write access to staging state.
// The scope (single service or all services) is determined at creation time.
type Writer interface {
	// WriteState writes state to storage.
	WriteState(ctx context.Context, state *staging.State) error
}

// DrainWriter combines drain and write operations.
type DrainWriter interface {
	Drainer
	Writer
}

// Lister provides list operations with type-parameterized return values.
// This generic interface allows ServiceReadWriter and GlobalStore to share
// the same method signatures with different return types.
type Lister[Entries any, Tags any] interface {
	// ListEntries returns staged entries.
	ListEntries(ctx context.Context) (Entries, error)
	// ListTags returns staged tag changes.
	ListTags(ctx context.Context) (Tags, error)
}

// ServiceLister is Lister specialized for a single service.
type ServiceLister = Lister[map[string]staging.Entry, map[string]staging.TagEntry]

// GlobalLister is Lister specialized for all services.
type GlobalLister = Lister[map[staging.Service]map[string]staging.Entry, map[staging.Service]map[string]staging.TagEntry]

// ServiceReadWriter provides read/write operations for a single service.
// The service is determined at creation time.
type ServiceReadWriter interface {
	ServiceLister
	// GetEntry retrieves a staged entry by name.
	GetEntry(ctx context.Context, name string) (*staging.Entry, error)
	// GetTag retrieves staged tag changes by name.
	GetTag(ctx context.Context, name string) (*staging.TagEntry, error)
	// StageEntry adds or updates a staged entry.
	StageEntry(ctx context.Context, name string, entry staging.Entry) error
	// StageTag adds or updates staged tag changes.
	StageTag(ctx context.Context, name string, tagEntry staging.TagEntry) error
	// UnstageEntry removes a staged entry.
	UnstageEntry(ctx context.Context, name string) error
	// UnstageTag removes staged tag changes.
	UnstageTag(ctx context.Context, name string) error
	// UnstageAll removes all staged changes for this service.
	UnstageAll(ctx context.Context) error
}

// ServiceStore provides full access to a single service's staging storage.
type ServiceStore interface {
	DrainWriter
	ServiceReadWriter
}

// GlobalStore provides bulk operations across all services.
type GlobalStore interface {
	DrainWriter
	GlobalLister
	// UnstageAll removes all staged changes across all services.
	UnstageAll(ctx context.Context) error
}

// FileStoreFactory creates file stores for staging state.
type FileStoreFactory interface {
	// Service returns a store for a specific service.
	Service(service staging.Service) DrainWriter
	// Global returns a store for all services.
	Global() DrainWriter
	// Exists checks if any stash file exists.
	Exists() (bool, error)
	// Encrypted checks if any stash file is encrypted.
	Encrypted() (bool, error)
	// Delete deletes all stash files.
	Delete() error
}

// AgentStoreFactory creates agent stores for staging state.
type AgentStoreFactory interface {
	// Service returns a store for a specific service.
	Service(service staging.Service) ServiceStore
	// Global returns a store for all services.
	Global() GlobalStore
	// Ping checks if the agent daemon is running.
	Ping(ctx context.Context) error
	// Start ensures the agent daemon is running, starting it if necessary.
	Start(ctx context.Context) error
}

// HintedServiceUnstager provides unstage operations with hints for context-aware shutdown messages.
type HintedServiceUnstager interface {
	// UnstageEntryWithHint removes a staged entry with an operation hint.
	UnstageEntryWithHint(ctx context.Context, name string, hint string) error
	// UnstageTagWithHint removes staged tag changes with an operation hint.
	UnstageTagWithHint(ctx context.Context, name string, hint string) error
	// UnstageAllWithHint removes all staged changes with an operation hint.
	UnstageAllWithHint(ctx context.Context, hint string) error
}

// HintedGlobalUnstager provides global unstage operations with hints.
type HintedGlobalUnstager interface {
	// UnstageAllWithHint removes all staged changes across all services with an operation hint.
	UnstageAllWithHint(ctx context.Context, hint string) error
}

// =============================================================================
// Legacy interfaces (kept for backward compatibility during migration)
// TODO: Remove after migration is complete
// =============================================================================

// LegacyReadOperator provides read-only access to individual staging entries.
//
// Deprecated: Use ServiceReadWriter instead.
type LegacyReadOperator interface {
	GetEntry(ctx context.Context, service staging.Service, name string) (*staging.Entry, error)
	GetTag(ctx context.Context, service staging.Service, name string) (*staging.TagEntry, error)
	ListEntries(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error)
	ListTags(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error)
}

// LegacyWriteOperator provides write access to individual staging entries.
//
// Deprecated: Use ServiceReadWriter instead.
type LegacyWriteOperator interface {
	StageEntry(ctx context.Context, service staging.Service, name string, entry staging.Entry) error
	StageTag(ctx context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error
	UnstageEntry(ctx context.Context, service staging.Service, name string) error
	UnstageTag(ctx context.Context, service staging.Service, name string) error
	UnstageAll(ctx context.Context, service staging.Service) error
}

// LegacyReadWriteOperator combines read and write access to staging entries.
//
// Deprecated: Use ServiceReadWriter instead.
type LegacyReadWriteOperator interface {
	LegacyReadOperator
	LegacyWriteOperator
}

// LegacyDrainer provides bulk read access to staging state.
//
// Deprecated: Use Drainer instead.
type LegacyDrainer interface {
	Drain(ctx context.Context, service staging.Service, keep bool) (*staging.State, error)
}

// LegacyWriter provides bulk write access to staging state.
//
// Deprecated: Use Writer instead.
type LegacyWriter interface {
	WriteState(ctx context.Context, service staging.Service, state *staging.State) error
}

// LegacyFileStore combines drain and write operations for file storage.
//
// Deprecated: Use FileStoreFactory instead.
type LegacyFileStore interface {
	LegacyDrainer
	LegacyWriter
}

// LegacyAgentStore provides full access to agent storage including drain/write operations.
//
// Deprecated: Use AgentStoreFactory instead.
type LegacyAgentStore interface {
	LegacyReadWriteOperator
	LegacyDrainer
	LegacyWriter
	Ping(ctx context.Context) error
	Start(ctx context.Context) error
}

// LegacyHintedUnstager provides unstage operations with hints for context-aware shutdown messages.
//
// Deprecated: Use HintedServiceUnstager instead.
type LegacyHintedUnstager interface {
	UnstageEntryWithHint(ctx context.Context, service staging.Service, name string, hint string) error
	UnstageTagWithHint(ctx context.Context, service staging.Service, name string, hint string) error
	UnstageAllWithHint(ctx context.Context, service staging.Service, hint string) error
}

// =============================================================================
// Type aliases for backward compatibility
// =============================================================================

// ReadOperator is an alias for LegacyReadOperator.
//
// Deprecated: Use ServiceReadWriter instead.
type ReadOperator = LegacyReadOperator

// WriteOperator is an alias for LegacyWriteOperator.
//
// Deprecated: Use ServiceReadWriter instead.
type WriteOperator = LegacyWriteOperator

// ReadWriteOperator is an alias for LegacyReadWriteOperator.
//
// Deprecated: Use ServiceReadWriter instead.
type ReadWriteOperator = LegacyReadWriteOperator

// FileStore is an alias for LegacyFileStore.
//
// Deprecated: Use FileStoreFactory instead.
type FileStore = LegacyFileStore

// AgentStore is an alias for LegacyAgentStore.
//
// Deprecated: Use AgentStoreFactory instead.
type AgentStore = LegacyAgentStore

// HintedUnstager is an alias for LegacyHintedUnstager.
//
// Deprecated: Use HintedServiceUnstager instead.
type HintedUnstager = LegacyHintedUnstager
