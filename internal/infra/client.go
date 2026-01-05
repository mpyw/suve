// Package infra provides AWS client initialization.
package infra

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

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

// AWSIdentity contains AWS account ID and region.
type AWSIdentity struct {
	AccountID string
	Region    string
}

// GetAWSIdentity retrieves the current AWS account ID and region.
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

	return &AWSIdentity{
		AccountID: aws.ToString(output.Account),
		Region:    cfg.Region,
	}, nil
}
