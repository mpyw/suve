package transition

import (
	"maps"

	"github.com/mpyw/suve/internal/maputil"
)

// StagedTags represents the staged tag changes.
// Tags are stored as diff operations rather than final state.
// Current remote values are checked at staging time for auto-skip.
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
