package staging

// Parser provides name/spec parsing without provider access.
// Use this interface when only parsing is needed (e.g., status, add commands).
type Parser interface {
	ServiceStrategy

	// ParseName parses and validates a name, returning only the base name without version specifiers.
	// Returns an error if version specifiers are present.
	ParseName(input string) (string, error)

	// ParseSpec parses a version spec string.
	// Returns the base name and whether a version/shift was specified.
	ParseSpec(input string) (name string, hasVersion bool, err error)
}

// ParserFactory creates a Parser without a provider client.
type ParserFactory func() Parser
