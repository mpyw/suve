package infra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAWSConfigProfiles(t *testing.T) {
	// Cannot use t.Parallel() because subtests use t.Setenv

	t.Run("parses sso_account_id", func(t *testing.T) {
		configContent := `
[default]
sso_account_id = 111111111111

[profile production]
sso_account_id = 222222222222

[profile staging]
sso_account_id = 333333333333
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)

		profiles := parseAWSConfigProfiles()

		assert.Equal(t, "111111111111", profiles["default"])
		assert.Equal(t, "222222222222", profiles["production"])
		assert.Equal(t, "333333333333", profiles["staging"])
	})

	t.Run("parses role_arn", func(t *testing.T) {
		configContent := `
[profile assume-role]
role_arn = arn:aws:iam::444444444444:role/MyRole
source_profile = default

[profile cross-account]
role_arn = arn:aws:iam::555555555555:role/CrossAccount
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)

		profiles := parseAWSConfigProfiles()

		assert.Equal(t, "444444444444", profiles["assume-role"])
		assert.Equal(t, "555555555555", profiles["cross-account"])
	})

	t.Run("sso_account_id takes precedence over role_arn", func(t *testing.T) {
		configContent := `
[profile mixed]
sso_account_id = 666666666666
role_arn = arn:aws:iam::777777777777:role/ShouldBeIgnored
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)

		profiles := parseAWSConfigProfiles()

		assert.Equal(t, "666666666666", profiles["mixed"])
	})

	t.Run("handles missing config file", func(t *testing.T) {
		t.Setenv("AWS_CONFIG_FILE", "/nonexistent/path/config")

		profiles := parseAWSConfigProfiles()

		assert.Nil(t, profiles)
	})

	t.Run("ignores profiles without account info", func(t *testing.T) {
		configContent := `
[profile with-account]
sso_account_id = 888888888888

[profile without-account]
region = ap-northeast-1
output = json
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)

		profiles := parseAWSConfigProfiles()

		assert.Equal(t, "888888888888", profiles["with-account"])
		_, exists := profiles["without-account"]
		assert.False(t, exists)
	})

	t.Run("handles DEFAULT section case-insensitively", func(t *testing.T) {
		configContent := `
[DEFAULT]
sso_account_id = 999999999999
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)

		profiles := parseAWSConfigProfiles()

		assert.Equal(t, "999999999999", profiles["default"])
	})
}

func TestFindProfileByAccountID(t *testing.T) {
	// Cannot use t.Parallel() because subtests use t.Setenv

	t.Run("finds matching profile", func(t *testing.T) {
		configContent := `
[profile production]
sso_account_id = 123456789012

[profile staging]
sso_account_id = 234567890123
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")

		profile := findProfileByAccountID("123456789012")

		assert.Equal(t, "production", profile)
	})

	t.Run("returns empty when no match", func(t *testing.T) {
		configContent := `
[profile production]
sso_account_id = 123456789012
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")

		profile := findProfileByAccountID("999999999999")

		assert.Equal(t, "", profile)
	})

	t.Run("prefers AWS_PROFILE when it matches", func(t *testing.T) {
		configContent := `
[profile alpha]
sso_account_id = 123456789012

[profile beta]
sso_account_id = 123456789012
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_PROFILE", "beta")
		t.Setenv("AWS_DEFAULT_PROFILE", "")

		profile := findProfileByAccountID("123456789012")

		assert.Equal(t, "beta", profile)
	})

	t.Run("ignores AWS_PROFILE when account does not match", func(t *testing.T) {
		configContent := `
[profile wrong-account]
sso_account_id = 111111111111

[profile correct-account]
sso_account_id = 123456789012
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_PROFILE", "wrong-account")
		t.Setenv("AWS_DEFAULT_PROFILE", "")

		profile := findProfileByAccountID("123456789012")

		assert.Equal(t, "correct-account", profile)
	})

	t.Run("returns deterministic result with multiple matches", func(t *testing.T) {
		configContent := `
[profile zebra]
sso_account_id = 123456789012

[profile alpha]
sso_account_id = 123456789012

[profile beta]
sso_account_id = 123456789012
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")

		// Should always return "alpha" (first alphabetically)
		for range 10 {
			profile := findProfileByAccountID("123456789012")
			assert.Equal(t, "alpha", profile)
		}
	})

	t.Run("prefers AWS_DEFAULT_PROFILE when AWS_PROFILE not set", func(t *testing.T) {
		configContent := `
[profile alpha]
sso_account_id = 123456789012

[profile beta]
sso_account_id = 123456789012
`
		configPath := createTempConfig(t, configContent)
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "beta")

		profile := findProfileByAccountID("123456789012")

		assert.Equal(t, "beta", profile)
	})

	t.Run("returns empty when config file missing", func(t *testing.T) {
		t.Setenv("AWS_CONFIG_FILE", "/nonexistent/path/config")
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")

		profile := findProfileByAccountID("123456789012")

		assert.Equal(t, "", profile)
	})
}

func TestGetAWSConfigPath(t *testing.T) {
	// Cannot use t.Parallel() because subtests use t.Setenv

	t.Run("uses AWS_CONFIG_FILE if set", func(t *testing.T) {
		t.Setenv("AWS_CONFIG_FILE", "/custom/path/config")

		path := getAWSConfigPath()

		assert.Equal(t, "/custom/path/config", path)
	})

	t.Run("uses default path if AWS_CONFIG_FILE not set", func(t *testing.T) {
		t.Setenv("AWS_CONFIG_FILE", "")

		path := getAWSConfigPath()

		home, err := os.UserHomeDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, ".aws", "config"), path)
	})
}

// createTempConfig creates a temporary AWS config file and returns its path.
func createTempConfig(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	err := os.WriteFile(configPath, []byte(content), 0o600)
	require.NoError(t, err)
	return configPath
}
