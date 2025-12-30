// Package aws provides AWS client initialization.
package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// LoadConfig loads the default AWS configuration.
func LoadConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx)
}

// NewSSMClient creates a new SSM client using the default configuration.
func NewSSMClient(ctx context.Context) (*ssm.Client, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return ssm.NewFromConfig(cfg), nil
}

// NewSMClient creates a new Secrets Manager client using the default configuration.
func NewSMClient(ctx context.Context) (*secretsmanager.Client, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return secretsmanager.NewFromConfig(cfg), nil
}
