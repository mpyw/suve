// Package aws provides AWS client initialization.
package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Clients holds initialized AWS service clients.
type Clients struct {
	SSM            *ssm.Client
	SecretsManager *secretsmanager.Client
}

// NewClients creates new AWS clients using the default configuration.
func NewClients(ctx context.Context) (*Clients, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &Clients{
		SSM:            ssm.NewFromConfig(cfg),
		SecretsManager: secretsmanager.NewFromConfig(cfg),
	}, nil
}
