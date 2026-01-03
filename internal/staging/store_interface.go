package staging

// StoreReader provides read-only access to staging state.
type StoreReader interface {
	// Get retrieves a staged entry.
	// Deprecated: Use GetEntry instead.
	Get(service Service, name string) (*Entry, error)
	// GetEntry retrieves a staged entry.
	GetEntry(service Service, name string) (*Entry, error)
	// GetTag retrieves staged tag changes.
	GetTag(service Service, name string) (*TagEntry, error)
	// List returns all staged entries for a service.
	// Deprecated: Use ListEntries instead.
	List(service Service) (map[Service]map[string]Entry, error)
	// ListEntries returns all staged entries for a service.
	ListEntries(service Service) (map[Service]map[string]Entry, error)
	// ListTags returns all staged tag changes for a service.
	ListTags(service Service) (map[Service]map[string]TagEntry, error)
	// Load loads the current staging state from disk.
	Load() (*State, error)
}

// StoreWriter provides write access to staging state.
type StoreWriter interface {
	// Stage adds or updates a staged entry.
	// Deprecated: Use StageEntry instead.
	Stage(service Service, name string, entry Entry) error
	// StageEntry adds or updates a staged entry.
	StageEntry(service Service, name string, entry Entry) error
	// StageTag adds or updates staged tag changes.
	StageTag(service Service, name string, tagEntry TagEntry) error
	// Unstage removes a staged entry.
	// Deprecated: Use UnstageEntry instead.
	Unstage(service Service, name string) error
	// UnstageEntry removes a staged entry.
	UnstageEntry(service Service, name string) error
	// UnstageTag removes staged tag changes.
	UnstageTag(service Service, name string) error
	// UnstageAll removes all staged changes for a service.
	UnstageAll(service Service) error
}

// StoreReadWriter combines read and write access to staging state.
type StoreReadWriter interface {
	StoreReader
	StoreWriter
}
