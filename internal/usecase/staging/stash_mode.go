package staging

// StashMode determines how to handle conflicts when the destination already has data.
type StashMode int

const (
	// StashModeMerge combines source data with existing destination data.
	// Later entries win on key conflicts.
	StashModeMerge StashMode = iota
	// StashModeOverwrite replaces existing destination data with source data.
	StashModeOverwrite
)
