package lifecycle

import "context"

// Pinger checks if the agent is running without starting it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Starter ensures the agent is running, starting it if necessary.
type Starter interface {
	Start(ctx context.Context) error
}

// ExecuteWrite executes a write command that requires the agent to be running.
// It ensures the agent is started before executing the action.
// The cmd parameter is used for type safety to ensure only write commands are passed.
func ExecuteWrite[T any](
	ctx context.Context,
	starter Starter,
	_ WriteCommand,
	action func() (T, error),
) (T, error) {
	var zero T

	if err := starter.Start(ctx); err != nil {
		return zero, err
	}

	return action()
}

// ExecuteRead executes a read command that checks if the agent is running first.
// If the agent is not running (Ping fails), it returns Result with NothingStaged=true.
// The cmd parameter is used for type safety to ensure only read commands are passed.
func ExecuteRead[T any](
	ctx context.Context,
	pinger Pinger,
	_ ReadCommand,
	action func() (T, error),
) (Result[T], error) {
	if err := pinger.Ping(ctx); err != nil {
		// Ping failure means agent not running = nothing staged.
		// This is not an error condition, so return nil error.
		return Result[T]{NothingStaged: true}, nil //nolint:nilerr
	}

	value, err := action()
	if err != nil {
		return Result[T]{}, err
	}

	return Result[T]{Value: value}, nil
}

// ExecuteFile executes a file-only command that doesn't require the agent.
// The cmd parameter is used for type safety to ensure only file commands are passed.
func ExecuteFile[T any](
	_ context.Context,
	_ FileCommand,
	action func() (T, error),
) (T, error) {
	return action()
}
