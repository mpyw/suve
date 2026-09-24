// Package maputil provides utilities for working with maps.
package maputil

import (
	"cmp"
	"iter"
	"maps"
	"slices"
)

// SortedKeys returns an iterator over the keys of m in ascending order. The
// ~map[K]V constraint also accepts defined map types such as Set.
func SortedKeys[M ~map[K]V, K cmp.Ordered, V any](m M) iter.Seq[K] {
	return slices.Values(slices.Sorted(maps.Keys(m)))
}
