package secret

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/model"
)

// CreateClient is the interface for the create use case.
type CreateClient interface {
	// CreateSecret creates a new secret.
	CreateSecret(ctx context.Context, secret *model.Secret) (*model.SecretWriteResult, error)
}

// CreateInput holds input for the create use case.
type CreateInput struct {
	Name        string
	Value       string
	Description string
}

// CreateOutput holds the result of the create use case.
type CreateOutput struct {
	Name      string
	VersionID string
	ARN       string
}

// CreateUseCase executes create operations.
type CreateUseCase struct {
	Client CreateClient
}

// Execute runs the create use case.
func (u *CreateUseCase) Execute(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	secret := &model.Secret{
		Name:        input.Name,
		Value:       input.Value,
		Description: input.Description,
	}

	result, err := u.Client.CreateSecret(ctx, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return &CreateOutput{
		Name:      result.Name,
		VersionID: result.Version,
		ARN:       result.ARN,
	}, nil
}
