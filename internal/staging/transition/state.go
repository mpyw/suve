package transition

// EntryState represents the current state of a staged entry.
type EntryState struct {
	CurrentValue *string          // nil means non-existing remotely
	StagedState  EntryStagedState // Current staging state
}

// EntryStagedState represents the staging state of an entry.
// This is a sealed interface - only the types defined in this package implement it.
type EntryStagedState interface {
	isEntryStagedState()
}

// EntryStagedStateNotStaged represents an entry that is not staged.
type EntryStagedStateNotStaged struct{}

func (EntryStagedStateNotStaged) isEntryStagedState() {}

// EntryStagedStateCreate represents an entry staged for creation.
type EntryStagedStateCreate struct {
	DraftValue string
}

func (EntryStagedStateCreate) isEntryStagedState() {}

// EntryStagedStateUpdate represents an entry staged for update.
type EntryStagedStateUpdate struct {
	DraftValue string
}

func (EntryStagedStateUpdate) isEntryStagedState() {}

// EntryStagedStateDelete represents an entry staged for deletion.
type EntryStagedStateDelete struct{}

func (EntryStagedStateDelete) isEntryStagedState() {}
