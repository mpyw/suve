// These are state.go's tests: they spell the sealed-interface marker methods,
// which are private to the state namespace.
//declscope:namespace state

package transition

import "testing"

// Test interface marker methods for coverage.
func TestEntryStagedState_Marker(t *testing.T) {
	t.Parallel()
	// These tests exist to cover the sealed interface marker methods
	var _ EntryStagedState = EntryStagedStateNotStaged{}

	var _ EntryStagedState = EntryStagedStateCreate{}

	var _ EntryStagedState = EntryStagedStateUpdate{}

	var _ EntryStagedState = EntryStagedStateDelete{}

	// Call the marker methods directly
	EntryStagedStateNotStaged{}.isEntryStagedState()
	EntryStagedStateCreate{DraftValue: "test"}.isEntryStagedState()
	EntryStagedStateUpdate{DraftValue: "test"}.isEntryStagedState()
	EntryStagedStateDelete{}.isEntryStagedState()
}
