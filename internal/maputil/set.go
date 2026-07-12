package maputil

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Set is a generic set type that serializes to/from JSON arrays.
type Set[T comparable] map[T]struct{}

// NewSet creates a Set from values.
func NewSet[T comparable](values ...T) Set[T] {
	s := make(Set[T], len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}

	return s
}

// Add adds a value to the set.
func (s Set[T]) Add(value T) {
	s[value] = struct{}{}
}

// Remove removes a value from the set.
func (s Set[T]) Remove(value T) {
	delete(s, value)
}

// Contains returns true if the value is in the set.
func (s Set[T]) Contains(value T) bool {
	_, ok := s[value]

	return ok
}

// Len returns the number of elements in the set.
func (s Set[T]) Len() int {
	return len(s)
}

// Values returns all values as a slice.
func (s Set[T]) Values() []T {
	return slices.Collect(maps.Keys(s))
}

// MarshalJSON implements json.Marshaler.
func (s Set[T]) MarshalJSON() ([]byte, error) {
	values := s.Values()

	// Emit an empty array (not null) for an empty/nil set so staged-state files
	// round-trip stably.
	if len(values) == 0 {
		return []byte("[]"), nil
	}

	// Deterministic order so staged-state files (which embed a Set in
	// TagEntry.Remove) don't churn byte-for-byte across otherwise-identical
	// writes. T is only comparable, not ordered, so sort by string form.
	slices.SortFunc(values, func(a, b T) int {
		return cmp.Compare(fmt.Sprint(a), fmt.Sprint(b))
	})

	return json.Marshal(values)
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *Set[T]) UnmarshalJSON(data []byte) error {
	var values []T
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}

	*s = NewSet(values...)

	return nil
}
