package dialogs

import (
	"charm.land/bubbles/v2/key"
	huh "charm.land/huh/v2"
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

// newModeField builds the staged/immediate mode toggle for the huh-form dialogs
// (entry/tag). Its option labels are the same `(•) Stage    ( ) Apply
// immediately` radio the custom delete dialog draws via modeLabel, so every
// dialog presents the mode toggle identically (issue #654 mock). Bound to a bool
// (true = staged), it cycles inline with ←/→.
func newModeField(staged *bool) huh.Field {
	return huh.NewSelect[bool]().Key("mode").Title("Mode").Inline(true).
		Options(
			huh.NewOption(modeLabel(true), true),
			huh.NewOption(modeLabel(false), false),
		).Value(staged)
}

// clipName renders an entry name with its App Configuration namespace badge (a
// bare name for the null/default namespace and every other provider).
func clipName(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return name + " @" + namespace
}
