// Package updatecheck provides a notify-only, non-blocking, opt-out check for
// newer releases of suve on GitHub.
//
// The public entry point is Notice, which returns a one-line update notice (or
// an empty string when there is nothing to report). It is deliberately
// side-effect-light: it only reads and writes a small cache file under
// ~/.suve, and it is safe and silent on ANY error (network, parse, filesystem).
// It never panics, never blocks meaningfully (a short HTTP timeout bounds the
// single network call, which itself happens at most once per 24h), and never
// surfaces an error to the caller.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"
)

const (
	// envOptOut disables the update check entirely when set to a non-empty value.
	envOptOut = "SUVE_NO_UPDATE_CHECK"

	// cacheTTL is how long a fetched result is trusted before checking again.
	cacheTTL = 24 * time.Hour

	// httpTimeout bounds the single network call so the check never blocks
	// meaningfully.
	httpTimeout = 2 * time.Second

	baseDirName   = ".suve"
	cacheFileName = "update-check.json"

	// releasesAPIURL is the GitHub API endpoint for the latest release.
	releasesAPIURL = "https://api.github.com/repos/mpyw/suve/releases/latest"
	// releasesPageURL is the human-facing releases page shown in the notice.
	releasesPageURL = "https://github.com/mpyw/suve/releases/latest"

	// devVersion is the sentinel version of local/dev builds, which never nag.
	devVersion = "dev"
)

// cacheEntry is the on-disk cache format:
//
//	{"checked_at": "<RFC3339>", "latest_version": "vX.Y.Z"}
type cacheEntry struct {
	CheckedAt     time.Time `json:"checked_at"`     //nolint:tagliatelle // stable on-disk cache schema
	LatestVersion string    `json:"latest_version"` //nolint:tagliatelle // stable on-disk cache schema
}

// checker holds the injectable dependencies used by Notice. Production code
// uses defaultChecker; tests construct their own to avoid touching the network,
// the real home directory, or the real clock.
type checker struct {
	now         func() time.Time
	lookupEnv   func(string) (string, bool)
	cachePath   func() (string, error)
	fetchLatest func(ctx context.Context) (string, error)
}

// Notice returns a one-line update notice when a newer release of mpyw/suve is
// available, or "" when there is nothing to report, the check is disabled, the
// build is a dev build, or any error occurs. It is safe to call on every
// invocation: the result is cached for 24h, so at most one network call is made
// per day.
func Notice(ctx context.Context, current string) string {
	return defaultChecker().notice(ctx, current)
}

// defaultChecker returns a checker wired to the real environment, filesystem,
// clock, and a short-timeout HTTP client.
func defaultChecker() *checker {
	client := &http.Client{Timeout: httpTimeout}

	return &checker{
		now:       time.Now,
		lookupEnv: os.LookupEnv,
		cachePath: defaultCachePath,
		fetchLatest: func(ctx context.Context) (string, error) {
			return fetchLatestRelease(ctx, client)
		},
	}
}

// notice implements the logic described on Notice. It never returns an error;
// any failure collapses to "".
func (c *checker) notice(ctx context.Context, current string) string {
	// 1. Opt-out via environment variable.
	if v, ok := c.lookupEnv(envOptOut); ok && v != "" {
		return ""
	}

	// 2. Dev builds and unparseable versions never nag.
	if current == devVersion {
		return ""
	}

	cur := normalize(current)
	if !semver.IsValid(cur) {
		return ""
	}

	// 3. Resolve the latest version (cache first, then network).
	latest := c.resolveLatest(ctx)
	if latest == "" {
		return ""
	}

	latest = normalize(latest)
	if !semver.IsValid(latest) {
		return ""
	}

	// 4. Compare and, if newer, build the notice.
	if semver.Compare(latest, cur) <= 0 {
		return ""
	}

	return fmt.Sprintf(
		"A new version of suve is available: %s -> %s  (%s)",
		cur, latest, releasesPageURL,
	)
}

// resolveLatest returns the latest known version tag. It uses a fresh cache
// entry (younger than cacheTTL) without any network access; otherwise it
// fetches from GitHub and refreshes the cache. On fetch failure it records a
// short-lived negative marker so retries are suppressed within the TTL. Any
// error yields "".
func (c *checker) resolveLatest(ctx context.Context) string {
	path, pathErr := c.cachePath()
	if pathErr == nil {
		if entry, ok := readCache(path); ok {
			if c.now().Sub(entry.CheckedAt) < cacheTTL {
				return entry.LatestVersion
			}
		}
	}

	latest, err := c.fetchLatest(ctx)
	if err != nil || latest == "" {
		// On fetch failure, record a short-lived negative marker (an empty
		// LatestVersion, treated as "checked, nothing to report") so a failed
		// probe suppresses retries within the TTL rather than paying the HTTP
		// timeout on every invocation.
		if pathErr == nil {
			_ = writeCache(path, cacheEntry{CheckedAt: c.now()})
		}

		return ""
	}

	if pathErr == nil {
		// Best-effort cache write; errors are ignored on purpose.
		_ = writeCache(path, cacheEntry{CheckedAt: c.now(), LatestVersion: latest})
	}

	return latest
}

// normalize ensures a version tag carries a leading "v" so it is comparable
// with golang.org/x/mod/semver.
func normalize(v string) string {
	if v == "" {
		return v
	}

	if v[0] == 'v' || v[0] == 'V' {
		return "v" + v[1:]
	}

	return "v" + v
}

// defaultCachePath returns ~/.suve/update-check.json.
func defaultCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, baseDirName, cacheFileName), nil
}

// readCache reads and parses the cache file. It returns ok=false on any error
// (missing file, malformed JSON, etc.).
func readCache(path string) (cacheEntry, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's home dir, not attacker input
	if err != nil {
		return cacheEntry{}, false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}, false
	}

	return entry, true
}

// writeCache writes the cache entry to path, creating parent directories as
// needed. Errors are returned so the caller can ignore them explicitly.
func writeCache(path string, entry cacheEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { //nolint:mnd // owner-only directory permissions
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600) //nolint:mnd // owner-only file permissions
}

// fetchLatestRelease fetches the latest release tag from the GitHub API. It
// returns an error on network failure, non-200 responses, or malformed JSON.
func fetchLatestRelease(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPIURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"` //nolint:tagliatelle // GitHub API response schema
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	return payload.TagName, nil
}
