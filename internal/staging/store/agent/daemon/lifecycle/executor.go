package lifecycle

import (
	"context"

	"github.com/mpyw/suve/internal/staging/store"
)

// ExecuteWrite executes a write command that requires the agent to be running.
// It ensures the agent is started before executing the action.
// The cmd parameter is used for type safety to ensure only write commands are passed.
func ExecuteWrite[T any](
	ctx context.Context,
	starter store.Starter,
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
	pinger store.Pinger,
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

// ExecuteWrite0 is like ExecuteWrite but for actions that don't return a value.
func ExecuteWrite0(
	ctx context.Context,
	starter store.Starter,
	cmd WriteCommand,
	action func() error,
) error {
	_, err := ExecuteWrite(ctx, starter, cmd, func() (struct{}, error) {
		return struct{}{}, action()
	})

	return err
}

// ExecuteRead0 is like ExecuteRead but for actions that don't return a value.
func ExecuteRead0(
	ctx context.Context,
	pinger store.Pinger,
	cmd ReadCommand,
	action func() error,
) (Result0, error) {
	result, err := ExecuteRead(ctx, pinger, cmd, func() (struct{}, error) {
		return struct{}{}, action()
	})

	return Result0{NothingStaged: result.NothingStaged}, err
}

// ExecuteFile0 is like ExecuteFile but for actions that don't return a value.
func ExecuteFile0(
	ctx context.Context,
	cmd FileCommand,
	action func() error,
) error {
	_, err := ExecuteFile(ctx, cmd, func() (struct{}, error) {
		return struct{}{}, action()
	})

	return err
}
