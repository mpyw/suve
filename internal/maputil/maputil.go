// Package maputil provides utilities for working with maps.
package maputil

import (
	"cmp"
	"iter"
	"maps"
	"slices"

	"github.com/samber/lo"
)

// SortedKeys returns an iterator over the keys of m in ascending order. The
// ~map[K]V constraint also accepts defined map types such as Set.
func SortedKeys[M ~map[K]V, K cmp.Ordered, V any](m M) iter.Seq[K] {
	return slices.Values(slices.Sorted(maps.Keys(m)))
}

// SortedNames returns an iterator over the unique names extracted from items
// via getName, in ascending order.
func SortedNames[T any](items []T, getName func(T) string) iter.Seq[string] {
	return SortedKeys(NewSet(lo.Map(items, func(item T, _ int) string {
		return getName(item)
	})...))
}
