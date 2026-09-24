//declscope:namespace browser
//
// The key bindings are dispatched by update.go and listed by help.go.
// One Bubble Tea Model is one unit however many files hold it, so these
// files share browser.go's namespace rather than each claiming their own.

// key.go holds the browser's key bindings. update.go dispatches on them and
// help.go lists them, so they belong to neither.

package browser

import (
	"charm.land/bubbles/v2/key"
)

// Page-local key bindings not present in the global map.
//
//nolint:gochecknoglobals // immutable page-local bindings
var (
	prefixKey    = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prefix"))
	filterKey    = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	valuesKey    = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "values"))
	recursiveKey = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "recursive/refresh"))
	revealKey    = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "reveal"))
	compareKey   = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compare"))
	// spaceKey is only for key matching; its help is built per-service by spaceHelp
	// so the "namespace" label shows only on App Configuration.
	spaceKey = key.NewBinding(key.WithKeys("space"))

	// List-width resize keys (the keyboard counterpart to dragging the divider):
	// ] widens the list, [ narrows it, each stepping within the clamps (#784).
	widenKey  = key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "widen list"))
	narrowKey = key.NewBinding(key.WithKeys("["), key.WithHelp("[", "narrow list"))

	// Mutation keys: open the create/edit/delete/tag/restore dialogs.
	newKey     = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new"))
	editKey    = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	deleteKey  = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))
	tagKey     = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tag"))
	restoreKey = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore"))

	// Help-only bindings: they carry no new keys the reducer dispatches on (the
	// real movement/enter/esc live in the global keys.Map), but give the help bar
	// context-appropriate labels — "↑/↓ move" over the raw up/down entries, and an
	// enter/esc whose label reflects the focused pane (history vs list vs a
	// focused filter field).
	moveKey     = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "move"))
	historyKey  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "history"))
	diffPickKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "diff"))
	backListKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "list"))
	// applyInputKey advertises committing a focused prefix/filter field. Enter and
	// esc are identical here (both blur the field and reload — handleInputKey), so
	// the help shows one honest "apply" binding rather than a false esc="cancel".
	applyInputKey = key.NewBinding(key.WithKeys("enter", "esc"), key.WithHelp("enter/esc", "apply"))
)
