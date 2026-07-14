//nolint:testpackage // white-box tests exercise unexported checker internals
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestNotice_UnwritableCache_InProcessFallbackLimitsProbing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// A cache path whose parent is a regular file: writeCache always fails and
	// readCache never finds a fresh entry, so only the in-process fallback can
	// bound probing.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	path := filepath.Join(blocker, "update-check.json")

	var (
		memo      cacheEntry
		memoValid bool
	)

	fetches := 0
	c := &checker{
		now:       fixedNow(now),
		lookupEnv: noEnv,
		cachePath: func() (string, error) { return path, nil },
		fetchLatest: func(context.Context) (string, error) {
			fetches++

			return "", errors.New("boom")
		},
		readMemo:  func() (cacheEntry, bool) { return memo, memoValid },
		writeMemo: func(e cacheEntry) { memo, memoValid = e, true },
	}

	const calls = 5
	for range calls {
		assert.Empty(t, c.notice(context.Background(), "v1.2.3"))
	}

	// The on-disk marker never persisted, but the in-process fallback still
	// suppresses retries after the first probe.
	_, ok := readCache(path)
	assert.False(t, ok, "unwritable cache must not persist a marker")
	assert.Equal(t, 1, fetches, "in-process fallback must bound probing to once per process")
}

func TestDefaultChecker_Wired(t *testing.T) {
	t.Parallel()

	c := defaultChecker()
	require.NotNil(t, c)
	assert.NotNil(t, c.now)
	assert.NotNil(t, c.lookupEnv)
	assert.NotNil(t, c.cachePath)
	assert.NotNil(t, c.fetchLatest)
	assert.NotNil(t, c.readMemo, "production wires the in-process fallback")
	assert.NotNil(t, c.writeMemo, "production wires the in-process fallback")
}

func TestProcMemo_RoundTrip(t *testing.T) { //nolint:paralleltest // mutates process-wide procMemo
	t.Cleanup(func() {
		procMemoMu.Lock()
		procMemo, procMemoValid = cacheEntry{}, false
		procMemoMu.Unlock()
	})

	if _, ok := readProcMemo(); ok {
		procMemoMu.Lock()
		procMemo, procMemoValid = cacheEntry{}, false
		procMemoMu.Unlock()
	}

	entry := cacheEntry{CheckedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), LatestVersion: "v1.2.3"}
	writeProcMemo(entry)

	got, ok := readProcMemo()
	require.True(t, ok)
	assert.Equal(t, entry, got)
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
		// Pre-release current versions (e.g. a v2.0.0-alpha.1 build) must compare
		// with SemVer 2.0 precedence and never nag a pre-release user to
		// "downgrade" to the last stable — /releases/latest excludes pre-releases,
		// so the last stable is what fetchLatest returns for such a build.
		{name: "prerelease current, last stable is older -> no nag", current: "v2.0.0-alpha.1", latest: "v1.6.1", wantNotice: false},
		{name: "prerelease current, final release is newer -> nag", current: "v2.0.0-alpha.1", latest: "v2.0.0", wantNotice: true},
		{name: "prerelease current, later prerelease -> nag", current: "v2.0.0-alpha.1", latest: "v2.0.0-alpha.2", wantNotice: true},
		{name: "prerelease current equals latest -> no nag", current: "v2.0.0-alpha.1", latest: "v2.0.0-alpha.1", wantNotice: false},
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

// withReleasesAPIURL points the releases API endpoint at u for the duration of
// the test and restores it afterward. Tests using it must not run in parallel:
// releasesAPIURL is a process-wide seam.
func withReleasesAPIURL(t *testing.T, u string) {
	t.Helper()

	prev := releasesAPIURL
	releasesAPIURL = u

	t.Cleanup(func() { releasesAPIURL = prev })
}

func TestFetchLatestRelease_Valid(t *testing.T) { //nolint:paralleltest // mutates the process-wide releasesAPIURL seam
	var gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")

		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9"}`))
	}))
	t.Cleanup(srv.Close)

	withReleasesAPIURL(t, srv.URL)

	got, err := fetchLatestRelease(context.Background(), srv.Client())
	require.NoError(t, err)
	assert.Equal(t, "v9.9.9", got)
	assert.Equal(t, "application/vnd.github+json", gotAccept, "request must advertise the GitHub JSON media type")
}

func TestFetchLatestRelease_Non200(t *testing.T) { //nolint:paralleltest // mutates the process-wide releasesAPIURL seam
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	withReleasesAPIURL(t, srv.URL)

	_, err := fetchLatestRelease(context.Background(), srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status: 404")
}

func TestFetchLatestRelease_MalformedJSON(t *testing.T) { //nolint:paralleltest // mutates the process-wide releasesAPIURL seam
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	t.Cleanup(srv.Close)

	withReleasesAPIURL(t, srv.URL)

	_, err := fetchLatestRelease(context.Background(), srv.Client())
	require.Error(t, err)
}

func TestFetchLatestRelease_RequestBuildError(t *testing.T) { //nolint:paralleltest // mutates the process-wide releasesAPIURL seam
	// An unparseable URL makes http.NewRequestWithContext fail before any I/O.
	withReleasesAPIURL(t, "://bad")

	_, err := fetchLatestRelease(context.Background(), http.DefaultClient)
	require.Error(t, err)
}

func TestFetchLatestRelease_NetworkError(t *testing.T) { //nolint:paralleltest // mutates the process-wide releasesAPIURL seam
	// A server closed before the request forces client.Do to fail.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	withReleasesAPIURL(t, url)

	_, err := fetchLatestRelease(context.Background(), http.DefaultClient)
	require.Error(t, err)
}

func TestNotice_EndToEnd_RealFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9"}`))
	}))
	t.Cleanup(srv.Close)

	withReleasesAPIURL(t, srv.URL)

	// Isolate HOME so defaultCachePath resolves under a temp dir with no cache,
	// forcing the real fetch through defaultChecker's fetchLatest closure.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(envOptOut, "") // ensure the check is not opted out

	got := Notice(context.Background(), "v1.0.0")
	assert.Contains(t, got, "v1.0.0 -> v9.9.9")

	// The fetched result must be cached under ~/.suve.
	entry, ok := readCache(filepath.Join(home, baseDirName, cacheFileName))
	require.True(t, ok, "a successful fetch must persist the cache")
	assert.Equal(t, "v9.9.9", entry.LatestVersion)
}

func TestDefaultCachePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := defaultCachePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, baseDirName, cacheFileName), path)
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "lowercase v preserved", in: "v1.2.3", want: "v1.2.3"},
		{name: "uppercase V normalized", in: "V1.2.3", want: "v1.2.3"},
		{name: "bare version gains v", in: "1.2.3", want: "v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, normalize(tt.in))
		})
	}
}

func TestWriteCache_MkdirAllError(t *testing.T) {
	t.Parallel()

	// A parent that is a regular file makes MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	err := writeCache(filepath.Join(blocker, cacheFileName), cacheEntry{CheckedAt: time.Now()})
	require.Error(t, err)
}
