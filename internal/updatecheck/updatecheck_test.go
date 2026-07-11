//nolint:testpackage // white-box tests exercise unexported checker internals
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedNow returns a clock function pinned to t.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// noEnv reports every environment variable as unset.
func noEnv(string) (string, bool) { return "", false }

// errFetch is a fetch func that always fails.
func errFetch(context.Context) (string, error) {
	return "", errors.New("boom")
}

func TestNotice_OptOut(t *testing.T) {
	t.Parallel()

	fetched := false
	c := &checker{
		now:       fixedNow(time.Now()),
		lookupEnv: func(k string) (string, bool) { return "1", k == envOptOut },
		cachePath: func() (string, error) { return "", errors.New("no cache") },
		fetchLatest: func(context.Context) (string, error) {
			fetched = true

			return "v9.9.8", nil
		},
	}

	assert.Empty(t, c.notice(context.Background(), "v1.0.0"))
	assert.False(t, fetched, "opt-out must not attempt a fetch")
}

func TestNotice_DevBuild(t *testing.T) {
	t.Parallel()

	fetched := false
	c := &checker{
		now:       fixedNow(time.Now()),
		lookupEnv: noEnv,
		cachePath: func() (string, error) { return "", errors.New("no cache") },
		fetchLatest: func(context.Context) (string, error) {
			fetched = true

			return "v9.9.7", nil
		},
	}

	assert.Empty(t, c.notice(context.Background(), "dev"))
	assert.False(t, fetched, "dev build must not attempt a fetch")
}

func TestNotice_UnparseableVersion(t *testing.T) {
	t.Parallel()

	c := &checker{
		now:         fixedNow(time.Now()),
		lookupEnv:   noEnv,
		cachePath:   func() (string, error) { return "", errors.New("no cache") },
		fetchLatest: func(context.Context) (string, error) { return "v9.9.9", nil },
	}

	assert.Empty(t, c.notice(context.Background(), "not-a-version"))
}

func TestNotice_FreshCache_NoFetch(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		cached string
		want   string
	}{
		{name: "newer cached yields notice", cached: "v1.3.0", want: "A new version of suve is available: v1.2.3 -> v1.3.0  (" + releasesPageURL + ")"},
		{name: "equal cached yields nothing", cached: "v1.2.3", want: ""},
		{name: "older cached yields nothing", cached: "v1.0.0", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeTempCache(t, cacheEntry{
				CheckedAt:     now.Add(-1 * time.Hour), // fresh (< 24h)
				LatestVersion: tt.cached,
			})

			fetched := false
			c := &checker{
				now:       fixedNow(now),
				lookupEnv: noEnv,
				cachePath: func() (string, error) { return path, nil },
				fetchLatest: func(context.Context) (string, error) {
					fetched = true

					return "v2.0.0", nil
				},
			}

			assert.Equal(t, tt.want, c.notice(context.Background(), "v1.2.3"))
			assert.False(t, fetched, "fresh cache must not trigger a fetch")
		})
	}
}

func TestNotice_StaleCache_FetchesAndWrites(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	path := writeTempCache(t, cacheEntry{
		CheckedAt:     now.Add(-48 * time.Hour), // stale (> 24h)
		LatestVersion: "v1.0.0",
	})

	c := &checker{
		now:         fixedNow(now),
		lookupEnv:   noEnv,
		cachePath:   func() (string, error) { return path, nil },
		fetchLatest: func(context.Context) (string, error) { return "v1.5.0", nil },
	}

	got := c.notice(context.Background(), "v1.2.3")
	assert.Contains(t, got, "v1.2.3 -> v1.5.0")

	// Cache must be refreshed with the fetched value and current time.
	entry, ok := readCache(path)
	require.True(t, ok)
	assert.Equal(t, "v1.5.0", entry.LatestVersion)
	assert.WithinDuration(t, now, entry.CheckedAt, time.Second)
}

func TestNotice_MissingCache_FetchesAndWrites(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "nested", "update-check.json")

	c := &checker{
		now:         fixedNow(now),
		lookupEnv:   noEnv,
		cachePath:   func() (string, error) { return path, nil },
		fetchLatest: func(context.Context) (string, error) { return "v2.0.0", nil },
	}

	got := c.notice(context.Background(), "v1.2.3")
	assert.Contains(t, got, "v1.2.3 -> v2.0.0")

	entry, ok := readCache(path)
	require.True(t, ok)
	assert.Equal(t, "v2.0.0", entry.LatestVersion)
}

func TestNotice_FetchError_NoCrash(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "update-check.json")

	c := &checker{
		now:         fixedNow(now),
		lookupEnv:   noEnv,
		cachePath:   func() (string, error) { return path, nil },
		fetchLatest: errFetch,
	}

	assert.Empty(t, c.notice(context.Background(), "v1.2.3"))

	// A failed fetch records a short-lived negative marker (empty
	// LatestVersion) so subsequent invocations within the TTL do not retry.
	entry, ok := readCache(path)
	require.True(t, ok, "failed fetch must record a negative marker")
	assert.Empty(t, entry.LatestVersion, "negative marker carries no version")
	assert.WithinDuration(t, now, entry.CheckedAt, time.Second)
}

func TestNotice_FetchError_SuppressesRetriesWithinTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "update-check.json")

	fetches := 0
	c := &checker{
		now:       fixedNow(now),
		lookupEnv: noEnv,
		cachePath: func() (string, error) { return path, nil },
		fetchLatest: func(context.Context) (string, error) {
			fetches++

			return "", errors.New("boom")
		},
	}

	const calls = 5
	for range calls {
		assert.Empty(t, c.notice(context.Background(), "v1.2.3"))
	}

	assert.Equal(t, 1, fetches, "a failed probe must be attempted at most once within the TTL")
}

func TestNotice_CachePathError_StillFetches(t *testing.T) {
	t.Parallel()

	c := &checker{
		now:         fixedNow(time.Now()),
		lookupEnv:   noEnv,
		cachePath:   func() (string, error) { return "", errors.New("no home") },
		fetchLatest: func(context.Context) (string, error) { return "v3.0.0", nil },
	}

	assert.Contains(t, c.notice(context.Background(), "v1.2.3"), "v1.2.3 -> v3.0.0")
}

func TestNotice_NormalizesTags(t *testing.T) {
	t.Parallel()

	// current without leading "v", latest with leading "v".
	c := &checker{
		now:         fixedNow(time.Now()),
		lookupEnv:   noEnv,
		cachePath:   func() (string, error) { return "", errors.New("no cache") },
		fetchLatest: func(context.Context) (string, error) { return "v1.3.0", nil },
	}

	assert.Contains(t, c.notice(context.Background(), "1.2.3"), "v1.2.3 -> v1.3.0")
}

func TestSemverComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		current    string
		latest     string
		wantNotice bool
	}{
		{name: "newer minor", current: "v1.2.3", latest: "v1.3.0", wantNotice: true},
		{name: "newer major", current: "v1.2.3", latest: "v2.0.0", wantNotice: true},
		{name: "newer patch", current: "v1.2.3", latest: "v1.2.4", wantNotice: true},
		{name: "equal", current: "v1.2.3", latest: "v1.2.3", wantNotice: false},
		{name: "older", current: "v1.2.3", latest: "v1.2.2", wantNotice: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &checker{
				now:         fixedNow(time.Now()),
				lookupEnv:   noEnv,
				cachePath:   func() (string, error) { return "", errors.New("no cache") },
				fetchLatest: func(context.Context) (string, error) { return tt.latest, nil },
			}

			got := c.notice(context.Background(), tt.current)
			if tt.wantNotice {
				assert.NotEmpty(t, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

func TestReadCache_Malformed(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "update-check.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	_, ok := readCache(path)
	assert.False(t, ok)
}

func TestCacheRoundTrip_RFC3339(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "update-check.json")
	checkedAt := time.Date(2026, 7, 4, 10, 30, 0, 0, time.UTC)
	require.NoError(t, writeCache(path, cacheEntry{CheckedAt: checkedAt, LatestVersion: "v1.2.3"}))

	// Verify the on-disk JSON uses RFC3339 for checked_at.
	raw, err := os.ReadFile(path) //nolint:gosec // test-controlled temp path
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "2026-07-04T10:30:00Z", m["checked_at"])
	assert.Equal(t, "v1.2.3", m["latest_version"])
}

// writeTempCache writes entry to a temp cache file and returns its path.
func writeTempCache(t *testing.T, entry cacheEntry) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "update-check.json")
	require.NoError(t, writeCache(path, entry))

	return path
}
