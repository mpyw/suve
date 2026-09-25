// In-package tests of profile.go's profile lookup.
//declscope:namespace profile

package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActiveProfile is the #1003 regression: the target names the profile in
// use (AWS_PROFILE, else AWS_DEFAULT_PROFILE) and never guesses another profile
// from ~/.aws/config that merely shares the account.
func TestActiveProfile(t *testing.T) {
	// A config where another profile carries a role_arn in the caller's account:
	// the old account-id lookup picked it over the profile actually in use.
	configPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(configPath, []byte(`
[profile prod-admin]
region = us-east-1

[profile prod-readonly]
role_arn = arn:aws:iam::123456789012:role/ReadOnly
`), 0o600))

	tests := []struct {
		name           string
		profile        string
		defaultProfile string
		accessKeyID    string
		want           string
	}{
		{name: "AWS_PROFILE is shown as is", profile: "prod-admin", want: "prod-admin"},
		{name: "AWS_DEFAULT_PROFILE when AWS_PROFILE is unset", defaultProfile: "prod-admin", want: "prod-admin"},
		{name: "AWS_PROFILE wins over AWS_DEFAULT_PROFILE", profile: "a", defaultProfile: "b", want: "a"},
		{name: "no profile env: omitted, not guessed", want: ""},
		{name: "env access keys override any profile", profile: "prod-admin", accessKeyID: "AKIAEXAMPLE", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_CONFIG_FILE", configPath)
			t.Setenv("AWS_PROFILE", tc.profile)
			t.Setenv("AWS_DEFAULT_PROFILE", tc.defaultProfile)
			t.Setenv("AWS_ACCESS_KEY_ID", tc.accessKeyID)

			assert.Equal(t, tc.want, activeProfile())
		})
	}
}
