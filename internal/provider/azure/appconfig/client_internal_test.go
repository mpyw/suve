package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListSettingSelector_FiltersNullLabel guards the #352 fix: List must
// restrict to the null (default) label so it matches the single-key operations,
// rather than enumerating every label (a nil LabelFilter). The reserved value
// "\x00" is sent as label=%00, which App Configuration matches to label-less
// key-values.
func TestListSettingSelector_FiltersNullLabel(t *testing.T) {
	t.Parallel()

	sel := listSettingSelector()

	require.NotNil(t, sel.LabelFilter, "List must set a label filter, not enumerate all labels")
	assert.Equal(t, "\x00", *sel.LabelFilter)

	// No key filter: all keys, restricted only by label.
	assert.Nil(t, sel.KeyFilter)
}
