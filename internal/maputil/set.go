package maputil

import "encoding/json"

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
	values := make([]T, 0, len(s))
	for v := range s {
		values = append(values, v)
	}
	return values
}

// MarshalJSON implements json.Marshaler.
func (s Set[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Values())
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
