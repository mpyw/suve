// These are action.go's tests: they spell the sealed-interface marker methods,
// which are private to the action namespace.
//declscope:namespace action

package transition

import (
	"testing"

	"github.com/mpyw/suve/internal/maputil"
)

// Test interface marker methods for coverage.
func TestEntryAction_Marker(t *testing.T) {
	t.Parallel()
	// These tests exist to cover the sealed interface marker methods
	var _ EntryAction = EntryActionAdd{}

	var _ EntryAction = EntryActionEdit{}

	var _ EntryAction = EntryActionDelete{}

	var _ EntryAction = EntryActionReset{}

	// Call the marker methods directly
	EntryActionAdd{Value: "test"}.isEntryAction()
	EntryActionEdit{Value: "test"}.isEntryAction()
	EntryActionDelete{}.isEntryAction()
	EntryActionReset{}.isEntryAction()
}

func TestTagAction_Marker(t *testing.T) {
	t.Parallel()
	// These tests exist to cover the sealed interface marker methods
	var _ TagAction = TagActionTag{}

	var _ TagAction = TagActionUntag{}

	// Call the marker methods directly
	TagActionTag{Tags: map[string]string{}}.isTagAction()
	TagActionUntag{Keys: maputil.NewSet[string]()}.isTagAction()
}
