package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
)

// TestListSettingSelector_ForwardsFilter checks that the selector forwards the
// given LabelFilter verbatim (never a nil filter, which would enumerate every
// label) and sets no key filter. The store resolves the raw --namespace value
// into this filter via aznamespace.Filter.
func TestListSettingSelector_ForwardsFilter(t *testing.T) {
	t.Parallel()

	for _, filter := range []string{"\x00", "dev", "dev,prod", "dev*", "*"} {
		sel := listSettingSelector(filter)

		require.NotNil(t, sel.LabelFilter, "List must set a label filter, not enumerate all labels")
		assert.Equal(t, filter, *sel.LabelFilter)

		// No key filter: all keys, restricted only by label.
		assert.Nil(t, sel.KeyFilter)
	}
}

// TestListSettingSelector_NullLabelDefault guards the #352 behavior end to end:
// the empty namespace resolves to the null-label filter ("\x00", sent as
// label=%00), so List matches only label-less key-values rather than every
// label.
func TestListSettingSelector_NullLabelDefault(t *testing.T) {
	t.Parallel()

	sel := listSettingSelector(aznamespace.Filter(""))

	require.NotNil(t, sel.LabelFilter)
	assert.Equal(t, "\x00", *sel.LabelFilter)
}
