package lifecycle

// Result wraps the return value of a read command execution.
// It indicates whether the operation succeeded with a value, or if there was nothing staged.
type Result[T any] struct {
	// Value contains the result of the action when NothingStaged is false.
	Value T

	// NothingStaged indicates that the agent was not running, meaning no changes are staged.
	// When true, Value should be ignored.
	NothingStaged bool
}

// Result0 is like Result but without a value.
// Used by ExecuteRead0 for actions that don't return a value.
type Result0 struct {
	// NothingStaged indicates that the agent was not running, meaning no changes are staged.
	NothingStaged bool
}
