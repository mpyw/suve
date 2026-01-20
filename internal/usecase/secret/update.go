package secret

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/model"
)

// UpdateClient is the interface for the update use case.
type UpdateClient interface {
	// GetSecret retrieves a secret by name with optional version specifier.
	GetSecret(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error)
	// UpdateSecret updates the value of an existing secret.
	UpdateSecret(ctx context.Context, name string, value string) (*model.SecretWriteResult, error)
}

// UpdateInput holds input for the update use case.
type UpdateInput struct {
	Name  string
	Value string
	// Description is currently not supported through the provider interface.
	// AWS-specific description updates should be handled separately.
}

// UpdateOutput holds the result of the update use case.
type UpdateOutput struct {
	Name      string
	VersionID string
	ARN       string
}

// UpdateUseCase executes update operations.
type UpdateUseCase struct {
	Client UpdateClient
}

// GetCurrentValue fetches the current secret value.
func (u *UpdateUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	secret, err := u.Client.GetSecret(ctx, name, "", "")
	if err != nil {
		return "", err
	}

	return secret.Value, nil
}

// Execute runs the update use case.
func (u *UpdateUseCase) Execute(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	result, err := u.Client.UpdateSecret(ctx, input.Name, input.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return &UpdateOutput{
		Name:      result.Name,
		VersionID: result.Version,
		ARN:       result.ARN,
	}, nil
}
