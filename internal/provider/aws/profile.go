package aws

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/ini.v1"

	"github.com/mpyw/suve/internal/maputil"
)

// profileARNAccountIDRegex extracts AWS account ID (12 digits) from ARN strings.
var profileARNAccountIDRegex = regexp.MustCompile(`:(\d{12}):`)

// findProfileByAccountID searches ~/.aws/config for a profile matching the account ID.
// It checks sso_account_id and role_arn fields to verify the profile actually
// corresponds to the given account ID (from GetCallerIdentity).
//
// Logic:
// 1. Parse ~/.aws/config to build a map of profile -> account ID
// 2. If AWS_PROFILE is set and its account matches, use it
// 3. Otherwise, return the first profile that matches the account ID.
//
//declscope:package // identity.go names the caller's profile with it
func findProfileByAccountID(accountID string) string {
	profileAccounts := parseAWSConfigProfiles()
	if len(profileAccounts) == 0 {
		return ""
	}

	// If AWS_PROFILE is set, verify it matches the actual account ID
	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" {
		if profileAccounts[envProfile] == accountID {
			return envProfile
		}
	}

	if envProfile := os.Getenv("AWS_DEFAULT_PROFILE"); envProfile != "" {
		if profileAccounts[envProfile] == accountID {
			return envProfile
		}
	}

	// Search all profiles for a match (sorted for deterministic results)
	for profile := range maputil.SortedKeys(profileAccounts) {
		if profileAccounts[profile] == accountID {
			return profile
		}
	}

	return ""
}

// parseAWSConfigProfiles parses ~/.aws/config and returns a map of profile name to account ID.
// Account ID is extracted from sso_account_id or role_arn.
func parseAWSConfigProfiles() map[string]string {
	configPath := profileConfigPath()

	cfg, err := ini.Load(configPath)
	if err != nil {
		return nil
	}

	profiles := make(map[string]string)

	for _, section := range cfg.Sections() {
		name := section.Name()

		// Extract profile name from section name
		// Sections are either "default" or "profile <name>"
		var profileName string
		if strings.EqualFold(name, "default") {
			profileName = "default"
		} else if after, found := strings.CutPrefix(name, "profile "); found {
			profileName = after
		} else {
			continue
		}

		// Try sso_account_id first
		if key, err := section.GetKey("sso_account_id"); err == nil {
			profiles[profileName] = key.String()

			continue
		}

		// Try role_arn (extract account ID from ARN)
		if key, err := section.GetKey("role_arn"); err == nil {
			if matches := profileARNAccountIDRegex.FindStringSubmatch(key.String()); len(matches) == 2 { //nolint:mnd // regex capture groups
				profiles[profileName] = matches[1]
			}
		}
	}

	return profiles
}

// profileConfigPath returns the path to ~/.aws/config.
func profileConfigPath() string {
	if configFile := os.Getenv("AWS_CONFIG_FILE"); configFile != "" {
		return configFile
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".aws", "config")
}
