package internal

import "fmt"

// ApplyShift applies a shift to a base index and returns the target index.
// It validates that the target index is within bounds [0, length).
// baseIdx is the starting index (0-indexed), shift is how many versions to go back.
func ApplyShift(baseIdx, shift, length int) (int, error) {
	targetIdx := baseIdx + shift
	if targetIdx < 0 || targetIdx >= length {
		return 0, fmt.Errorf("version shift out of range: ~%d", shift)
	}

	return targetIdx, nil
}
