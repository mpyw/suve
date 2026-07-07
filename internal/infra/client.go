// Package infra provides AWS client initialization.
package infra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/logging"
	"github.com/samber/lo"
	"gopkg.in/ini.v1"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/maputil"
)

// arnAccountIDRegex extracts AWS account ID (12 digits) from ARN strings.
var arnAccountIDRegex = regexp.MustCompile(`:(\d{12}):`)

// debugLogger adapts the debug writer to smithy's logging.Logger so SDK
// request/response dumps share the unified "[suve debug ...]" line prefix with
// every other provider (multi-line HTTP dumps are prefixed on their first line
// only).
type debugLogger struct {
	cfg debug.Config
}

// Logf implements smithy logging.Logger.
func (l debugLogger) Logf(classification logging.Classification, format string, v ...any) {
	l.cfg.Logf("aws sdk %s: %s\n", classification, fmt.Sprintf(format, v...))
}

// LoadConfig loads the default AWS configuration. When debug is enabled on the
// context it turns on SDK request/response/retry logging (metadata only — the
// bodyless LogRequest/LogResponse modes never print secret values) plus config
// resolution warnings, and logs a one-line summary of the effective region,
// profile, and credentials source — the facts a user needs first when a command
// unexpectedly returns nothing (see #306).
func LoadConfig(ctx context.Context) (aws.Config, error) {
	d := debug.From(ctx)
	if !d.Enabled {
		return config.LoadDefaultConfig(ctx)
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithClientLogMode(aws.LogRequest|aws.LogResponse|aws.LogRetries),
		config.WithLogger(debugLogger{cfg: d}),
		config.WithLogConfigurationWarnings(true),
	)
	if err != nil {
		return cfg, err
	}

	logEffectiveConfig(ctx, d, cfg)

	return cfg, nil
}

// logEffectiveConfig emits the one-line effective-configuration summary under
// debug. Resolving the credentials source calls Retrieve, which is cached by
// the SDK's CredentialsCache, so the first API call would perform the same work
// anyway; a resolution failure is logged (with the reason) instead of being
// returned, so the command still fails at the API call exactly as it would
// without --debug.
func logEffectiveConfig(ctx context.Context, d debug.Config, cfg aws.Config) {
	profile := lo.CoalesceOrEmpty(os.Getenv("AWS_PROFILE"), os.Getenv("AWS_DEFAULT_PROFILE"), "default")

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		d.Logf("aws: region=%q profile=%q credentials resolution failed: %v\n", cfg.Region, profile, err)

		return
	}

	d.Logf("aws: region=%q profile=%q credentials-source=%s\n", cfg.Region, profile, creds.Source)
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
			if matches := arnAccountIDRegex.FindStringSubmatch(key.String()); len(matches) == 2 { //nolint:mnd // regex capture groups
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
