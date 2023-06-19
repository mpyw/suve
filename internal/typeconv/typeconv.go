package typeconv

func Ref[T any](value T) *T {
	return &value
}

func NonNilOrDefault[T any](ptr *T, defaultValue T) T {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

func NonNilOrEmpty[T any](ptr *T) T {
	var empty T
	return NonNilOrDefault(ptr, empty)
}
