package parallel_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
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

	t.Run("actually parallel", func(t *testing.T) {
		t.Parallel()

		entries := map[int]string{
			1: "a",
			2: "b",
			3: "c",
		}

		var (
			running       int32
			maxConcurrent int32
		)

		results := parallel.ExecuteMap(t.Context(), entries, func(_ context.Context, _ int, _ string) (bool, error) {
			current := atomic.AddInt32(&running, 1)
			// Update maxConcurrent if current is higher
			for {
				oldMax := atomic.LoadInt32(&maxConcurrent)
				if current <= oldMax || atomic.CompareAndSwapInt32(&maxConcurrent, oldMax, current) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&running, -1)

			return true, nil
		})

		require.Len(t, results, 3)
		// Should have run at least 2 concurrently
		assert.GreaterOrEqual(t, maxConcurrent, int32(2))
	})
}

func TestExecuteMapWithLimit(t *testing.T) {
	t.Parallel()

	t.Run("respects limit", func(t *testing.T) {
		t.Parallel()

		entries := map[int]string{
			1: "a",
			2: "b",
			3: "c",
			4: "d",
			5: "e",
		}

		var (
			maxConcurrent int32
			running       int32
		)

		results := parallel.ExecuteMapWithLimit(t.Context(), entries, 2, func(_ context.Context, _ int, _ string) (bool, error) {
			current := atomic.AddInt32(&running, 1)

			for {
				oldMax := atomic.LoadInt32(&maxConcurrent)
				if current <= oldMax || atomic.CompareAndSwapInt32(&maxConcurrent, oldMax, current) {
					break
				}
			}

			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&running, -1)

			return true, nil
		})

		require.Len(t, results, 5)
		// Should not exceed limit of 2
		assert.LessOrEqual(t, maxConcurrent, int32(2))
	})

	t.Run("limit of 1 is sequential", func(t *testing.T) {
		t.Parallel()

		entries := map[int]string{
			1: "a",
			2: "b",
			3: "c",
		}

		var (
			maxConcurrent int32
			running       int32
		)

		results := parallel.ExecuteMapWithLimit(t.Context(), entries, 1, func(_ context.Context, _ int, _ string) (bool, error) {
			current := atomic.AddInt32(&running, 1)

			for {
				oldMax := atomic.LoadInt32(&maxConcurrent)
				if current <= oldMax || atomic.CompareAndSwapInt32(&maxConcurrent, oldMax, current) {
					break
				}
			}

			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&running, -1)

			return true, nil
		})

		require.Len(t, results, 3)
		assert.Equal(t, int32(1), maxConcurrent)
	})
}

func TestDefaultLimit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 10, parallel.DefaultLimit)
}
