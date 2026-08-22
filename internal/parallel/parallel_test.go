package parallel_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/parallel"
)

func TestExecuteMap(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		entries := map[string]int{
			"a": 1,
			"b": 2,
			"c": 3,
		}

		results := parallel.ExecuteMap(t.Context(), entries, func(_ context.Context, _ string, value int) (int, error) {
			return value * 2, nil
		})

		require.Len(t, results, 3)
		assert.Equal(t, 2, results["a"].Value)
		require.NoError(t, results["a"].Err)
		assert.Equal(t, 4, results["b"].Value)
		require.NoError(t, results["b"].Err)
		assert.Equal(t, 6, results["c"].Value)
		assert.NoError(t, results["c"].Err)
	})

	t.Run("with errors", func(t *testing.T) {
		t.Parallel()

		entries := map[string]int{
			"a": 1,
			"b": 2,
		}

		results := parallel.ExecuteMap(t.Context(), entries, func(_ context.Context, key string, _ int) (int, error) {
			if key == "b" {
				return 0, errors.New("error for b")
			}

			return 42, nil
		})

		require.Len(t, results, 2)
		assert.Equal(t, 42, results["a"].Value)
		require.NoError(t, results["a"].Err)
		assert.Equal(t, 0, results["b"].Value)
		assert.EqualError(t, results["b"].Err, "error for b")
	})

	t.Run("empty map", func(t *testing.T) {
		t.Parallel()

		entries := map[string]int{}

		results := parallel.ExecuteMap(t.Context(), entries, func(_ context.Context, _ string, value int) (int, error) {
			return value, nil
		})

		assert.Empty(t, results)
	})

	// Inside a synctest bubble time is virtual: the clock only advances once EVERY
	// goroutine is durably blocked, so all three workers are guaranteed to have
	// reached the sleep before any of them wakes. The observed concurrency is
	// therefore exact (3 entries, well under DefaultLimit) rather than the "at
	// least 2" a wall-clock sleep could only ever assert without flaking, and the
	// sleep costs no real time.
	//
	// The workers sleep with time.Sleep, not synctest.Sleep: the latter appends a
	// synctest.Wait, which may not be called concurrently by multiple goroutines
	// in the same bubble. Virtual time comes from the bubble, not from the helper.
	t.Run("actually parallel", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			entries := map[int]string{
				1: "a",
				2: "b",
				3: "c",
			}

			var running, maxConcurrent atomic.Int32

			results := parallel.ExecuteMap(t.Context(), entries, func(_ context.Context, _ int, _ string) (bool, error) {
				trackMax(running.Add(1), &maxConcurrent)
				time.Sleep(10 * time.Millisecond)
				running.Add(-1)

				return true, nil
			})

			require.Len(t, results, 3)
			assert.Equal(t, int32(3), maxConcurrent.Load(), "every entry under the limit runs concurrently")
		})
	})
}

// trackMax raises maxConcurrent to current when current is the new high-water
// mark, retrying until the CAS lands so a concurrent raise is never lost.
func trackMax(current int32, maxConcurrent *atomic.Int32) {
	for {
		oldMax := maxConcurrent.Load()
		if current <= oldMax || maxConcurrent.CompareAndSwap(oldMax, current) {
			return
		}
	}
}

func TestExecuteMapWithLimit(t *testing.T) {
	t.Parallel()

	// The bubble's clock makes the observed concurrency exact, so this asserts the
	// limit is REACHED as well as never exceeded — a plain `<= 2` would also pass
	// if the pool never got past one worker.
	t.Run("respects limit", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			entries := map[int]string{
				1: "a",
				2: "b",
				3: "c",
				4: "d",
				5: "e",
			}

			var running, maxConcurrent atomic.Int32

			results := parallel.ExecuteMapWithLimit(t.Context(), entries, 2, func(_ context.Context, _ int, _ string) (bool, error) {
				trackMax(running.Add(1), &maxConcurrent)
				time.Sleep(20 * time.Millisecond)
				running.Add(-1)

				return true, nil
			})

			require.Len(t, results, 5)
			assert.Equal(t, int32(2), maxConcurrent.Load(), "the pool saturates the limit but never exceeds it")
		})
	})

	t.Run("non-positive limit falls back to default without deadlocking", func(t *testing.T) {
		t.Parallel()

		for _, limit := range []int{0, -1} {
			t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
				t.Parallel()

				// A non-positive limit used to deadlock on errgroup's zero-capacity
				// semaphore. A synctest bubble fails the test as soon as its
				// goroutines are all blocked with no way to make progress, so the
				// regression is caught immediately — no watchdog goroutine, result
				// channel or wall-clock timeout needed to keep it from hanging the
				// suite.
				synctest.Test(t, func(t *testing.T) {
					entries := map[int]string{1: "a", 2: "b", 3: "c"}

					results := parallel.ExecuteMapWithLimit(t.Context(), entries, limit, func(_ context.Context, _ int, value string) (string, error) {
						return value, nil
					})

					require.Len(t, results, 3)
					assert.Equal(t, "a", results[1].Value)
					assert.Equal(t, "b", results[2].Value)
					assert.Equal(t, "c", results[3].Value)
				})
			})
		}
	})

	t.Run("limit of 1 is sequential", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			entries := map[int]string{
				1: "a",
				2: "b",
				3: "c",
			}

			var running, maxConcurrent atomic.Int32

			results := parallel.ExecuteMapWithLimit(t.Context(), entries, 1, func(_ context.Context, _ int, _ string) (bool, error) {
				trackMax(running.Add(1), &maxConcurrent)
				time.Sleep(5 * time.Millisecond)
				running.Add(-1)

				return true, nil
			})

			require.Len(t, results, 3)
			assert.Equal(t, int32(1), maxConcurrent.Load())
		})
	})
}

func TestDefaultLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 10, parallel.DefaultLimit)
}
