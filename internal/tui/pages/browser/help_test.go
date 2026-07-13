//nolint:testpackage // white-box: exercises the unexported focus/capability gating of the page KeyMap
package browser

import (
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
)

// helpDescs flattens a key map's short-help descriptions.
func helpDescs(km help.KeyMap) []string {
	descs := make([]string, 0, len(km.ShortHelp()))
	for _, b := range km.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}

	return descs
}

// fullHelpDescs flattens a key map's full-help descriptions across all columns.
func fullHelpDescs(km help.KeyMap) []string {
	var descs []string

	for _, col := range km.FullHelp() {
		for _, b := range col {
			descs = append(descs, b.Help().Desc)
		}
	}

	return descs
}

// TestHelpKeyMap_ShortHelpAdaptsToFocus pins that the short help follows the
// focused pane: the list advertises the mutation/filter keys; once enter moves
// focus into the version history it advertises the pane's own transitions
// (compare, back-to-list) instead.
func TestHelpKeyMap_ShortHelpAdaptsToFocus(t *testing.T) {
	t.Parallel()

	m := loadedHistoryModel(t)

	listHelp := helpDescs(m.HelpKeyMap())
	assert.Contains(t, listHelp, "edit", "list-focused help shows the mutation keys")
	assert.Contains(t, listHelp, "history", "list-focused help advertises enter→history")

	m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, focusHistory, m.focus, "enter moves focus into the history")

	historyHelp := helpDescs(m.HelpKeyMap())
	assert.NotEqual(t, listHelp, historyHelp, "the short help changes with the focused pane")
	assert.Contains(t, historyHelp, "compare", "history-focused help advertises compare")
	assert.Contains(t, historyHelp, "list", "history-focused help advertises esc→list")
	assert.NotContains(t, historyHelp, "edit", "history focus drops the list mutation keys")
}

// TestHelpKeyMap_FocusedFilterShowsCommit pins that while a header filter field
// is focused (CapturesInput), only the commit transition shows — the page's
// action keys would be typed as text, not dispatched. Enter and esc both commit
// here, so the help lists a single honest "apply" binding.
func TestHelpKeyMap_FocusedFilterShowsCommit(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	m, _ = update(t, m, keyPress('/'))
	require.Equal(t, focusFilter, m.focus, "`/` focuses the filter field")

	descs := helpDescs(m.HelpKeyMap())
	assert.Equal(t, []string{"apply"}, descs, "a focused filter shows only the commit binding")
}

// TestHelpKeyMap_FullHelpGatesOnCapability pins the capability gating of the
// full-help columns: a no-history/no-tags/no-restore service (App Config) omits
// compare/tag/restore, while a versioned service with tags and restore (AWS
// secret) includes them.
func TestHelpKeyMap_FullHelpGatesOnCapability(t *testing.T) {
	t.Parallel()

	appConfig := newModel(t, &stubSource{svcCap: appConfigCap()})
	appConfig, _ = update(t, appConfig, listLoadedMsg{
		seq: appConfig.listSeq, res: data.ListResult{Items: []data.Item{{Name: "k"}}},
	})

	secret := newModel(t, &stubSource{svcCap: awsSecretCap()})
	secret, _ = update(t, secret, listLoadedMsg{
		seq: secret.listSeq, res: data.ListResult{Items: []data.Item{{Name: "k"}}},
	})

	appConfigFull := fullHelpDescs(appConfig.HelpKeyMap())
	assert.NotContains(t, appConfigFull, "compare", "App Config has no version history, so no compare")
	assert.NotContains(t, appConfigFull, "restore", "App Config has no restore")

	secretFull := fullHelpDescs(secret.HelpKeyMap())
	assert.Contains(t, secretFull, "compare", "AWS secret is versioned, so compare is available")
	assert.Contains(t, secretFull, "tag", "AWS secret supports tags")
	assert.Contains(t, secretFull, "restore", "AWS secret supports restore")
}

// TestHelpKeyMap_ResizeKeysGatedOnTwoPane pins #784's help: the list resize keys
// (widen/narrow) are advertised in the two-pane layout and dropped once the panes
// stack (where they would do nothing).
func TestHelpKeyMap_ResizeKeysGatedOnTwoPane(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/k"}}}})

	// newModel sizes the model at 120 wide (two-pane): the resize keys show.
	twoPane := fullHelpDescs(m.HelpKeyMap())
	assert.Contains(t, twoPane, "widen list", "the two-pane help advertises widen")
	assert.Contains(t, twoPane, "narrow list", "the two-pane help advertises narrow")

	// Narrow the terminal below the two-pane threshold: the keys drop out.
	m.width = twoPaneMinWidth - 1
	stacked := fullHelpDescs(m.HelpKeyMap())
	assert.NotContains(t, stacked, "widen list", "the stacked help omits the resize keys")
}

// TestHelpKeyMap_NoStagingShortcut pins #785: the removed `S` staging jump is no
// longer advertised anywhere in the browser help (Staging stays reachable via the
// tab keys).
func TestHelpKeyMap_NoStagingShortcut(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsSecretCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "k"}}}})

	assert.NotContains(t, fullHelpDescs(m.HelpKeyMap()), "staging", "the broken S staging shortcut is gone (#785)")
}

// TestHelpKeyMap_LoadMoreGatedOnNextPage pins that load-more only appears while a
// next page is pending.
func TestHelpKeyMap_LoadMoreGatedOnNextPage(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsSecretCap()})

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "k"}}}})
	assert.NotContains(t, fullHelpDescs(m.HelpKeyMap()), "load more", "no next page → no load-more key")

	m, _ = update(t, m, listLoadedMsg{
		seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "k"}}, NextToken: "tok"},
	})
	assert.Contains(t, fullHelpDescs(m.HelpKeyMap()), "load more", "a pending next page surfaces load-more")
}
