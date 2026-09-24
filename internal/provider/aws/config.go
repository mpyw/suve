package aws

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
)

// LoadConfig loads the default AWS configuration. When debug is enabled on the
// context it turns on SDK request/response/retry logging plus config resolution
// warnings, and logs a one-line summary of the effective region, profile, and
// credentials source — the facts a user needs first when a command unexpectedly
// returns nothing (see #306). By default the bodyless LogRequest/LogResponse
// modes are used (metadata only, no secret values); --no-redaction switches to
// the WithBody modes so full request/response payloads are logged too.
func LoadConfig(ctx context.Context) (aws.Config, error) {
	d := debug.From(ctx)
	if !d.Enabled {
		return config.LoadDefaultConfig(ctx)
	}

	logMode := aws.LogRequest | aws.LogResponse | aws.LogRetries
	if d.NoRedaction {
		logMode = aws.LogRequestWithBody | aws.LogResponseWithBody | aws.LogRetries
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithClientLogMode(logMode),
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
