package transition

import (
	"maps"

	"github.com/mpyw/suve/internal/maputil"
)

// EntryState represents the current state of a staged entry.
type EntryState struct {
	CurrentValue *string          // nil means non-existing on AWS
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

// StagedTags represents the staged tag changes.
// Tags are stored as diff operations rather than final state.
// AWS current values are checked at staging time for auto-skip.
type StagedTags struct {
	ToSet   map[string]string   // Tags to add or update
	ToUnset maputil.Set[string] // Tag keys to remove
}

// IsEmpty returns true if there are no staged tag changes.
func (t StagedTags) IsEmpty() bool {
	return len(t.ToSet) == 0 && t.ToUnset.Len() == 0
}

// Clone returns a deep copy of the staged tags with initialized maps.
func (t StagedTags) Clone() StagedTags {
	toSet := maps.Clone(t.ToSet)
	if toSet == nil {
		toSet = make(map[string]string)
	}

	toUnset := maps.Clone(t.ToUnset)
	if toUnset == nil {
		toUnset = maputil.NewSet[string]()
	}

	return StagedTags{
		ToSet:   toSet,
		ToUnset: toUnset,
	}
}
