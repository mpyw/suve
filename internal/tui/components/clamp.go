package components

// clamp constrains v to [lo, hi]. lo is a parameter (rather than a hardcoded 0)
// so the shared list/history widgets read as a general clamp; today every caller
// passes 0, which is fine.
//
//nolint:unparam // general-purpose clamp shared by the list and history widgets
//declscope:package // shared by the list (list.go) and history (history.go) widgets
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}

	return max(lo, min(v, hi))
}
