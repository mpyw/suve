package staging

// ImportMode determines how to reconcile imported state with the existing
// working staging area.
type ImportMode int

const (
	// ImportModeMerge combines the imported state with the existing working
	// staging area. Later entries win on key conflicts.
	ImportModeMerge ImportMode = iota
	// ImportModeOverwrite replaces the existing working staging area with the
	// imported state.
	ImportModeOverwrite
)
