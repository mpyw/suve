//declscope:core
//declscope:package
//
// This file is, by design, the package's shared widget vocabulary: every
// declaration is a small binding, renderer, or metric consumed by several
// dialogs. It joins the core namespace (a "widgets" prefix on each name would
// add noise, not information) and everything it declares is package-wide.

package dialogs

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
)

// Shared navigation bindings for the custom (non-huh) dialogs (delete/restore
// hint lines and toggles).
//
//nolint:gochecknoglobals // immutable dialog-local bindings
var (
	navUp     = key.NewBinding(key.WithKeys("up", "k"))
	navDown   = key.NewBinding(key.WithKeys("down", "j", "tab"))
	navLeft   = key.NewBinding(key.WithKeys("left", "h"))
	navRight  = key.NewBinding(key.WithKeys("right", "l"))
	navDec    = key.NewBinding(key.WithKeys("-", "_"))
	navInc    = key.NewBinding(key.WithKeys("+", "="))
	navSelect = key.NewBinding(key.WithKeys("enter", "space"))
)

// checkbox renders a "[x]"/"[ ]" checkbox.
func checkbox(on bool) string {
	if on {
		return "[x]"
	}

	return "[ ]"
}

// modeLabel renders the staged-vs-immediate mode as radio-style options.
func modeLabel(staged bool) string {
	if staged {
		return "(•) Stage    ( ) Apply immediately"
	}

	return "( ) Stage    (•) Apply immediately"
}

// clipName renders an entry name with its App Configuration namespace badge (a
// bare name for the null/default namespace and every other provider).
func clipName(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return name + " @" + namespace
}

// requiredField builds a huh validator that rejects an empty/whitespace value.
func requiredField(label string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return stringError(label + " is required")
		}

		return nil
	}
}

// titleSpacerRows is the blank line the form dialogs draw between the title and
// the form body; it is reserved when budgeting the body's scrollable height.
const titleSpacerRows = 1

// minFormBody floors the embedded form's scrollable body height so a very short
// terminal still shows at least a field or two (the rest scrolls into view)
// rather than collapsing the form to nothing.
const minFormBody = 3

// buttonGap is the column gap between two side-by-side confirm buttons (the
// "    " separator), used to place the second button's hit region.
const buttonGap = 4

// regionClose is the close-hint region ID (shared by the error dialog and any
// confirm dialog whose close hint is clickable).
const regionClose = "close"

// pluralize renders "n singular"/"n plural".
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(n) + " " + plural
}
