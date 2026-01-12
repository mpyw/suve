package staging

import "context"

// StoreReadOperator provides read-only access to individual staging entries.
type StoreReadOperator interface {
	// GetEntry retrieves a staged entry.
	GetEntry(ctx context.Context, service Service, name string) (*Entry, error)
	// GetTag retrieves staged tag changes.
	GetTag(ctx context.Context, service Service, name string) (*TagEntry, error)
	// ListEntries returns all staged entries for a service.
	ListEntries(ctx context.Context, service Service) (map[Service]map[string]Entry, error)
	// ListTags returns all staged tag changes for a service.
	ListTags(ctx context.Context, service Service) (map[Service]map[string]TagEntry, error)
}

// StoreWriteOperator provides write access to individual staging entries.
type StoreWriteOperator interface {
	// StageEntry adds or updates a staged entry.
	StageEntry(ctx context.Context, service Service, name string, entry Entry) error
	// StageTag adds or updates staged tag changes.
	StageTag(ctx context.Context, service Service, name string, tagEntry TagEntry) error
	// UnstageEntry removes a staged entry.
	UnstageEntry(ctx context.Context, service Service, name string) error
	// UnstageTag removes staged tag changes.
	UnstageTag(ctx context.Context, service Service, name string) error
	// UnstageAll removes all staged changes for a service.
	UnstageAll(ctx context.Context, service Service) error
}

// StoreReadWriteOperator combines read and write access to staging entries.
type StoreReadWriteOperator interface {
	StoreReadOperator
	StoreWriteOperator
}

// StateDrainer provides bulk read access to staging state (for drain command).
type StateDrainer interface {
	// Drain retrieves the entire state from storage.
	// If keep is false, the source storage is cleared after reading.
	Drain(ctx context.Context, keep bool) (*State, error)
}

// StatePersister provides bulk write access to staging state (for persist command).
type StatePersister interface {
	// Persist saves the entire state to storage.
	// If keep is false, the source (memory) should be cleared by the caller.
	Persist(ctx context.Context, state *State) error
}

// StateIO combines drain and persist operations for file storage.
type StateIO interface {
	StateDrainer
	StatePersister
}

// Deprecated aliases for backward compatibility.
// TODO: Remove these after updating all references.
type (
	StoreReader     = StoreReadOperator
	StoreWriter     = StoreWriteOperator
	StoreReadWriter = StoreReadWriteOperator
)
