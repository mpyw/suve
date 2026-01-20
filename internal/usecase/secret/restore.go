package secret

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/model"
)

// RestoreClient is the interface for the restore use case.
type RestoreClient interface {
	// RestoreSecret restores a previously deleted secret.
	RestoreSecret(ctx context.Context, name string) (*model.SecretRestoreResult, error)
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
	result, err := u.Client.RestoreSecret(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to restore secret: %w", err)
	}

	return &RestoreOutput{
		Name: result.Name,
		ARN:  result.ARN,
	}, nil
}
