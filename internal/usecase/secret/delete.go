package secret

import (
	"context"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/model"
)

// DeleteClient is the interface for the delete use case.
type DeleteClient interface {
	// GetSecret retrieves a secret by name with optional version specifier.
	GetSecret(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error)
	// DeleteSecret deletes a secret.
	DeleteSecret(ctx context.Context, name string, forceDelete bool) (*model.SecretDeleteResult, error)
}

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name  string
	Force bool // Force immediate deletion
	// RecoveryWindow is not supported through the provider interface.
	// AWS-specific recovery window should be handled at the CLI/adapter level.
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
	secret, err := u.Client.GetSecret(ctx, name, "", "")
	if err != nil {
		// Treat any error as "not found" for simplicity
		return "", nil //nolint:nilerr // intentionally ignoring error to treat as not found
	}

	return secret.Value, nil
}

// Execute runs the delete use case.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	result, err := u.Client.DeleteSecret(ctx, input.Name, input.Force)
	if err != nil {
		return nil, fmt.Errorf("failed to delete secret: %w", err)
	}

	return &DeleteOutput{
		Name:         result.Name,
		DeletionDate: result.DeletionDate,
		ARN:          result.ARN,
	}, nil
}
