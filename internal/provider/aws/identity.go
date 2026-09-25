package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/samber/lo"
)

// Identity contains the AWS account ID, region, and profile name of the caller.
// Profile is "" when no profile env var names the one in use (see activeProfile).
type Identity struct {
	AccountID string
	Region    string
	Profile   string
}

// LoadIdentity retrieves the current AWS account ID, region, and profile name.
func LoadIdentity(ctx context.Context) (*Identity, error) {
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

	return &Identity{
		AccountID: accountID,
		Region:    cfg.Region,
		Profile:   activeProfile(),
	}, nil
}
