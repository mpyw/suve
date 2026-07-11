package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

// nonEmptyState returns a state with a single staged create so writeFile takes
// the real write path rather than the empty-state removal branch.
func nonEmptyState() *staging.State {
	s := staging.NewEmptyState()
	s.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("v"),
	}

	return s
}

// setInteractive overrides the TTY seam for the duration of the test.
func setInteractive(t *testing.T, interactive bool) {
	t.Helper()

	orig := isInteractiveFunc

	isInteractiveFunc = func() bool { return interactive }

	t.Cleanup(func() { isInteractiveFunc = orig })
}

// setAllowPlaintextEnv overrides the env seam so tests never touch the real
// process environment. Passing "" simulates the variable being unset.
func setAllowPlaintextEnv(t *testing.T, value string) {
	t.Helper()

	orig := lookupEnvFunc

	lookupEnvFunc = func(key string) (string, bool) {
		if key == EnvAllowPlaintext {
			if value == "" {
				return "", false
			}

			return value, true
		}

		return orig(key)
	}

	t.Cleanup(func() { lookupEnvFunc = orig })
}

// TestPlaintextGuard_Write covers the write-time guard across the matrix of
// interactivity, opt-in, the plaintext-fallback marker, and a keyed store. The
// guard must fire ONLY for an unencrypted working-store fallback in a
// non-interactive session with no consent; every other combination writes.
//
//nolint:paralleltest // mutates package-level isInteractiveFunc/lookupEnvFunc seams
func TestPlaintextGuard_Write(t *testing.T) {
	tests := []struct {
		name          string
		interactive   bool
		consent       string
		marker        bool
		keyed         bool
		wantBlocked   bool
		wantEncrypted bool // checked only when not blocked
	}{
		{"non-interactive fallback, no consent -> blocked", false, "", true, false, true, false},
		{"non-interactive fallback, env consent -> plaintext", false, "1", true, false, false, false},
		{"interactive fallback -> plaintext (warn-and-proceed)", true, "", true, false, false, false},
		{"non-interactive keyed store -> encrypted", false, "", false, true, false, true},
		{"non-interactive unmarked plaintext (export-like) -> plaintext", false, "", false, false, false, false},
	}

	for _, tt := range tests {
		//nolint:paralleltest // mutates package-level seams
		t.Run(tt.name, func(t *testing.T) {
			setInteractive(t, tt.interactive)
			setAllowPlaintextEnv(t, tt.consent)

			path := filepath.Join(t.TempDir(), "stage.json")
			store := NewStoreWithPath(path)
			store.plaintextFallback = tt.marker

			if tt.keyed {
				store.key = newTestKey()
			}

			err := store.WriteState(t.Context(), "", nonEmptyState())

			if tt.wantBlocked {
				require.ErrorIs(t, err, ErrPlaintextConsentRequired)

				_, statErr := os.Stat(path)
				require.ErrorIs(t, statErr, os.ErrNotExist, "no state file must be written when the guard fires")

				return
			}

			require.NoError(t, err)

			raw, readErr := os.ReadFile(path) //nolint:gosec // test temp path
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantEncrypted, crypt.IsEncrypted(raw))
		})
	}
}

// TestPlaintextGuard_ReadsNotGated confirms the guard is writes-only: an existing
// plaintext store is still readable non-interactively without consent.
//
//nolint:paralleltest // mutates package-level seams
func TestPlaintextGuard_ReadsNotGated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stage.json")

	// Seed a plaintext file via a consented write.
	setInteractive(t, false)
	setAllowPlaintextEnv(t, "1")

	seed := NewStoreWithPath(path)
	seed.plaintextFallback = true
	require.NoError(t, seed.WriteState(t.Context(), "", nonEmptyState()))

	// Read it back with consent withdrawn: reads must not fire the guard.
	setAllowPlaintextEnv(t, "")

	store := NewStoreWithPath(path)
	store.plaintextFallback = true

	got, err := store.Drain(t.Context(), "", true)
	require.NoError(t, err)
	assert.Equal(t, "v", lo.FromPtr(got.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}].Value))
}

// TestPlaintextConsentGranted_LenientParsing documents the truthiness rules for
// the opt-in env var.
//
//nolint:paralleltest // mutates package-level seams
func TestPlaintextConsentGranted_LenientParsing(t *testing.T) {
	cases := map[string]bool{
		"unset": false,
		"0":     false,
		"false": false,
		"1":     true,
		"true":  true,
		"yes":   true,
		"on":    true,
	}

	for value, want := range cases {
		//nolint:paralleltest // mutates package-level seams
		t.Run(value, func(t *testing.T) {
			env := value
			if value == "unset" {
				env = ""
			}

			setAllowPlaintextEnv(t, env)
			assert.Equal(t, want, plaintextConsentGranted())
		})
	}
}
