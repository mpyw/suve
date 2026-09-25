// In-package tests of detect.go's AWS shared-file probe.
//declscope:namespace detect

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAWSSharedFilesExist covers the fallback probe: either the shared
// credentials file or the shared config file counts (#1015: an SSO-only setup
// has just ~/.aws/config), each honoring its env override.
func TestAWSSharedFilesExist(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(existing, nil, 0o600))

	missing := filepath.Join(dir, "missing")

	tests := []struct {
		name        string
		credentials string
		config      string
		want        bool
	}{
		{name: "neither file", credentials: missing, config: missing, want: false},
		{name: "credentials file only", credentials: existing, config: missing, want: true},
		{name: "config file only (SSO)", credentials: missing, config: existing, want: true},
		{name: "a directory does not count", credentials: dir, config: missing, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", tc.credentials)
			t.Setenv("AWS_CONFIG_FILE", tc.config)

			assert.Equal(t, tc.want, awsSharedFilesExist())
		})
	}

	t.Run("defaults to ~/.aws", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		t.Setenv("AWS_CONFIG_FILE", "")

		assert.False(t, awsSharedFilesExist())

		require.NoError(t, os.MkdirAll(filepath.Join(home, ".aws"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(home, ".aws", "config"), nil, 0o600))

		assert.True(t, awsSharedFilesExist())
	})
}
