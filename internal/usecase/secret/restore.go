package secret

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// RestoreClient is the interface for the restore use case.
type RestoreClient interface {
	secretapi.RestoreSecretAPI
}

// RestoreInput holds input for the restore use case.
type RestoreInput struct {
	Name string
}

// RestoreOutput holds the result of the restore use case.
type RestoreOutput struct {
	Name string
	ARN  string
}

// RestoreUseCase executes restore operations.
type RestoreUseCase struct {
	Client RestoreClient
}

// Execute runs the restore use case.
func (u *RestoreUseCase) Execute(ctx context.Context, input RestoreInput) (*RestoreOutput, error) {
	result, err := u.Client.RestoreSecret(ctx, &secretapi.RestoreSecretInput{
		SecretId: lo.ToPtr(input.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to restore secret: %w", err)
	}

	return &RestoreOutput{
		Name: lo.FromPtr(result.Name),
		ARN:  lo.FromPtr(result.ARN),
	}, nil
}
