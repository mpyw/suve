package staging

// StoreReader provides read-only access to staging state.
type StoreReader interface {
	// Get retrieves a staged change.
	Get(service Service, name string) (*Entry, error)
	// List returns all staged changes for a service.
	List(service Service) (map[Service]map[string]Entry, error)
}

// StoreWriter provides write access to staging state.
type StoreWriter interface {
	// Stage adds or updates a staged change.
	Stage(service Service, name string, entry Entry) error
	// Unstage removes a staged change.
	Unstage(service Service, name string) error
	// UnstageAll removes all staged changes for a service.
	UnstageAll(service Service) error
}

// StoreReadWriter combines read and write access to staging state.
type StoreReadWriter interface {
	StoreReader
	StoreWriter
}
