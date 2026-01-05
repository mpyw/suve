package transition

import "github.com/mpyw/suve/internal/maputil"

// EntryAction represents an action on an entry.
// This is a sealed interface - only the types defined in this package implement it.
type EntryAction interface {
	isEntryAction()
}

// EntryActionAdd represents adding a new entry (create operation).
type EntryActionAdd struct {
	Value string
}

func (EntryActionAdd) isEntryAction() {}

// EntryActionEdit represents editing an existing entry (update operation).
type EntryActionEdit struct {
	Value string
}

func (EntryActionEdit) isEntryAction() {}

// EntryActionDelete represents deleting an entry.
type EntryActionDelete struct{}

func (EntryActionDelete) isEntryAction() {}

// EntryActionReset represents resetting (unstaging) an entry.
type EntryActionReset struct{}

func (EntryActionReset) isEntryAction() {}

// TagAction represents an action on tags.
// This is a sealed interface - only the types defined in this package implement it.
type TagAction interface {
	isTagAction()
}

// TagActionTag represents adding or updating tags.
// CurrentAWSTags is used to auto-skip tags that match AWS current values.
// Pass nil to disable auto-skip (e.g., when AWS tags couldn't be fetched).
type TagActionTag struct {
	Tags           map[string]string // Tags to add or update
	CurrentAWSTags map[string]string // Current AWS tag values for auto-skip (nil to disable)
}

func (TagActionTag) isTagAction() {}

// TagActionUntag represents removing tags.
// CurrentAWSTagKeys is used to auto-skip tag keys that don't exist on AWS.
// Pass nil to disable auto-skip (e.g., when AWS tags couldn't be fetched).
type TagActionUntag struct {
	Keys              maputil.Set[string] // Tag keys to remove
	CurrentAWSTagKeys maputil.Set[string] // Current AWS tag keys for auto-skip (nil to disable)
}

func (TagActionUntag) isTagAction() {}
