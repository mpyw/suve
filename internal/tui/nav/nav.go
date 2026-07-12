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

// OpenStaging asks the app to switch to the Staging tab (the browser's `S` jump).
type OpenStaging struct{}

// Reload asks the app to reload the active page after a mutation or staging
// action applied through a dialog (both the browser and the staging page react
// to it, refreshing their data and staged-count badges).
type Reload struct{}

// OpenApply asks the app to open the apply confirmation dialog for a set of
// services. Global marks the fan-out (apply-all) variant; EntryCount/TagCount are
// the staged totals across the targets, shown on the confirmation.
type OpenApply struct {
	Services   []string
	Global     bool
	EntryCount int
	TagCount   int
}

// OpenReset asks the app to open the reset confirmation dialog for a set of
// services. Global marks the reset-all variant.
type OpenReset struct {
	Services []string
	Global   bool
}

// OpenStagingDetail asks the app to push a full-diff page comparing an entry's
// remote value against its staged value (the staging page's `enter` detail),
// reusing the diff viewer for long values.
type OpenStagingDetail struct {
	Title    string
	OldLabel string
	NewLabel string
	OldValue string
	NewValue string
	// Secret masks both sides before diffing, so a secret detail never renders a
	// revealed value.
	Secret bool
}

// OpenEntryForm asks the app to open the create/edit dialog. Edit fixes the name
// and seeds the value/type/description from the selected entry; create seeds only
// the App Configuration namespace default (the concrete namespace being viewed).
type OpenEntryForm struct {
	Service     string
	Edit        bool
	Name        string
	Namespace   string
	Value       string
	TypeLabel   string
	Description string
}

// OpenDelete asks the app to open the delete-confirm dialog for the selected
// entry.
type OpenDelete struct {
	Service   string
	Name      string
	Namespace string
}

// OpenTag asks the app to open the tag add/remove dialog for the selected entry.
type OpenTag struct {
	Service   string
	Name      string
	Namespace string
}

// OpenRestore asks the app to open the restore dialog (a name input) for a
// soft-deleted entry.
type OpenRestore struct {
	Service string
	Name    string
}

// OpenError asks the app to open a plain error dialog (a blocked operation or a
// staging key-loss hard-fail).
type OpenError struct {
	Title   string
	Message string
}

// StagedCount reports the number of staged items a browser page counted for its
// service, so the app can total them into the Staging tab's count badge.
type StagedCount struct {
	Service string
	Count   int
}

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
	// The diff page learns secret-ness from the source's DiffContent, not from
	// here, so this request carries no secret flag.
	OldVersion string
	NewVersion string
}
