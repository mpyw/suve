package staging

import "context"

// StoreReader provides read-only access to staging state.
type StoreReader interface {
	// GetEntry retrieves a staged entry.
	GetEntry(ctx context.Context, service Service, name string) (*Entry, error)
	// GetTag retrieves staged tag changes.
	GetTag(ctx context.Context, service Service, name string) (*TagEntry, error)
	// ListEntries returns all staged entries for a service.
	ListEntries(ctx context.Context, service Service) (map[Service]map[string]Entry, error)
	// ListTags returns all staged tag changes for a service.
	ListTags(ctx context.Context, service Service) (map[Service]map[string]TagEntry, error)
	// Load loads the current staging state from disk.
	Load(ctx context.Context) (*State, error)
}

// StoreWriter provides write access to staging state.
type StoreWriter interface {
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

// StoreReadWriter combines read and write access to staging state.
type StoreReadWriter interface {
	StoreReader
	StoreWriter
}
