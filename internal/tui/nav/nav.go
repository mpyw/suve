// Package nav holds the upward navigation messages a page emits to the app
// shell. Keeping them in a leaf package (imported by both the app and the pages)
// lets a child request navigation — open the diff page, jump to the staging
// tab, pop back — without importing the app, so children never mutate parent
// state directly (the crush/gh-dash root-model rule from the epic).
package nav

import (
	"github.com/mpyw/suve/internal/tui/data"
)

// PopPage asks the app to pop the top page off the page stack (e.g. Esc from the
// diff page returns to the browser).
type PopPage struct{}

// OpenStaging asks the app to switch to the Staging tab (the `S` jump; the
// staging page itself is a placeholder until Step 5).
type OpenStaging struct{}

// OpenDiff asks the app to push the diff page for two versions of one entry. It
// carries the data source and version identifiers so the app can build the diff
// page without knowing the browser's internals.
type OpenDiff struct {
	// Source is the read-path source the diff page fetches through.
	Source data.Source
	// Name is the entry name; Namespace is its App Configuration namespace (empty
	// otherwise).
	Name      string
	Namespace string
	// OldVersion / NewVersion are the raw provider version identifiers to diff.
	OldVersion string
	NewVersion string
	// Secret reports whether the diffed value is a secret (the diff page still
	// renders the diff, but the flag lets the app label the page consistently).
	Secret bool
}
