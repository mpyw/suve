// Package infra provides AWS client initialization.
package infra

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/samber/lo"
	"gopkg.in/ini.v1"

	"github.com/mpyw/suve/internal/maputil"
)

// arnAccountIDRegex extracts AWS account ID (12 digits) from ARN strings.
var arnAccountIDRegex = regexp.MustCompile(`:(\d{12}):`)

// LoadConfig loads the default AWS configuration.
func LoadConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx)
}

// NewParamClient creates a new SSM Parameter Store client using the default configuration.
func NewParamClient(ctx context.Context) (*ssm.Client, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return ssm.NewFromConfig(cfg), nil
}

// NewSecretClient creates a new Secrets Manager client using the default configuration.
func NewSecretClient(ctx context.Context) (*secretsmanager.Client, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return secretsmanager.NewFromConfig(cfg), nil
}

// AWSIdentity contains AWS account ID, region, and profile name.
type AWSIdentity struct {
	AccountID string
	Region    string
	Profile   string
}

// GetAWSIdentity retrieves the current AWS account ID, region, and profile name.
func GetAWSIdentity(ctx context.Context) (*AWSIdentity, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	stsClient := sts.NewFromConfig(cfg)

	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}

	accountID := lo.FromPtr(output.Account)

	return &AWSIdentity{
		AccountID: accountID,
		Region:    cfg.Region,
		Profile:   findProfileByAccountID(accountID),
	}, nil
}

// findProfileByAccountID searches ~/.aws/config for a profile matching the account ID.
// It checks sso_account_id and role_arn fields to verify the profile actually
// corresponds to the given account ID (from GetCallerIdentity).
//
// Logic:
// 1. Parse ~/.aws/config to build a map of profile -> account ID
// 2. If AWS_PROFILE is set and its account matches, use it
// 3. Otherwise, return the first profile that matches the account ID.
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
	for _, profile := range maputil.SortedKeys(profileAccounts) {
		if profileAccounts[profile] == accountID {
			return profile
		}
	}

	return ""
}

// parseAWSConfigProfiles parses ~/.aws/config and returns a map of profile name to account ID.
// Account ID is extracted from sso_account_id or role_arn.
func parseAWSConfigProfiles() map[string]string {
	configPath := getAWSConfigPath()

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
			if matches := arnAccountIDRegex.FindStringSubmatch(key.String()); len(matches) == 2 {
				profiles[profileName] = matches[1]
			}
		}
	}

	return profiles
}

// getAWSConfigPath returns the path to ~/.aws/config.
func getAWSConfigPath() string {
	if configFile := os.Getenv("AWS_CONFIG_FILE"); configFile != "" {
		return configFile
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".aws", "config")
}
