// Package parallel provides utilities for parallel execution of operations.
package parallel

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// DefaultLimit is the default concurrency limit for parallel operations.
const DefaultLimit = 10

// Result holds the result of a parallel operation.
type Result[T any] struct {
	Value T
	Err   error
}

// ExecuteMap runs fn for each key-value pair in entries concurrently.
// Results are collected in a map keyed by the same keys as entries.
// Individual errors are captured in Result.Err rather than failing the entire operation.
func ExecuteMap[K comparable, V any, R any](
	ctx context.Context,
	entries map[K]V,
	fn func(ctx context.Context, key K, value V) (R, error),
) map[K]*Result[R] {
	return ExecuteMapWithLimit(ctx, entries, DefaultLimit, fn)
}

// ExecuteMapWithLimit is like ExecuteMap but with a custom concurrency limit.
func ExecuteMapWithLimit[K comparable, V any, R any](
	ctx context.Context,
	entries map[K]V,
	limit int,
	fn func(ctx context.Context, key K, value V) (R, error),
) map[K]*Result[R] {
	results := make(map[K]*Result[R], len(entries))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)

	for key, value := range entries {
		g.Go(func() error {
			result, err := fn(gctx, key, value)
			mu.Lock()
			results[key] = &Result[R]{Value: result, Err: err}
			mu.Unlock()
			return nil // Don't fail the group on individual errors
		})
	}

	_ = g.Wait()
	return results
}
