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

// SortedNames extracts unique names from a slice using the provided getter function
// and returns them sorted in ascending order.
func SortedNames[T any](items []T, getName func(T) string) []string {
	names := make(map[string]struct{})
	for _, item := range items {
		names[getName(item)] = struct{}{}
	}

	return SortedKeys(names)
}
