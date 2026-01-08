package secret

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// DeleteClient is the interface for the delete use case.
type DeleteClient interface {
	secretapi.DeleteSecretAPI
	secretapi.GetSecretValueAPI
}

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name           string
	Force          bool  // Force immediate deletion
	RecoveryWindow int64 // Days before permanent deletion (7-30)
}

// DeleteOutput holds the result of the delete use case.
type DeleteOutput struct {
	Name         string
	DeletionDate *time.Time
	ARN          string
}

// DeleteUseCase executes delete operations.
type DeleteUseCase struct {
	Client DeleteClient
}

// GetCurrentValue fetches the current secret value for preview.
func (u *DeleteUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	out, err := u.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
		SecretId: lo.ToPtr(name),
	})
	if err != nil {
		if rnf := (*secretapi.ResourceNotFoundException)(nil); errors.As(err, &rnf) {
			return "", nil
		}
		return "", err
	}
	return lo.FromPtr(out.SecretString), nil
}

// Execute runs the delete use case.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	deleteInput := &secretapi.DeleteSecretInput{
		SecretId: lo.ToPtr(input.Name),
	}
	if input.Force {
		deleteInput.ForceDeleteWithoutRecovery = lo.ToPtr(true)
	} else if input.RecoveryWindow > 0 {
		deleteInput.RecoveryWindowInDays = lo.ToPtr(input.RecoveryWindow)
	}

	result, err := u.Client.DeleteSecret(ctx, deleteInput)
	if err != nil {
		return nil, fmt.Errorf("failed to delete secret: %w", err)
	}

	return &DeleteOutput{
		Name:         lo.FromPtr(result.Name),
		DeletionDate: result.DeletionDate,
		ARN:          lo.FromPtr(result.ARN),
	}, nil
}
