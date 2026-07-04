package internal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/mpyw/suve/internal/infra"
)

// NewParamClient initializes an SSM Parameter Store client, wrapping any
// initialization failure with a consistent, user-facing error message.
func NewParamClient(ctx context.Context) (*ssm.Client, error) {
	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return client, nil
}

// NewSecretClient initializes a Secrets Manager client, wrapping any
// initialization failure with a consistent, user-facing error message.
func NewSecretClient(ctx context.Context) (*secretsmanager.Client, error) {
	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return client, nil
}
