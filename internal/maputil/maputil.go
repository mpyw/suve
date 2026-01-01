// Package maputil provides utilities for working with maps.
package maputil

import (
	"cmp"
	"slices"

	"github.com/samber/lo"
)

// SortedKeys returns the keys of a map sorted in ascending order.
func SortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := lo.Keys(m)
	slices.Sort(keys)
	return keys
}
