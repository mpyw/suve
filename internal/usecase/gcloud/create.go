package gcloud

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// CreateInput holds input for the create use case.
type CreateInput struct {
	Name        string
	Value       string
	Description string
}

// CreateOutput holds the result of the create use case.
type CreateOutput struct {
	Name    string
	Version string
}

// CreateUseCase executes create operations.
type CreateUseCase struct {
	Writer provider.Writer
}

// Execute runs the create use case. It creates a new secret via the provider;
// if the secret already exists the provider returns a wrapped
// provider.ErrAlreadyExists and no overwrite occurs.
func (u *CreateUseCase) Execute(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	version, err := u.Writer.Create(ctx, input.Name, input.Value, domain.ValueTypeSecret, input.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return &CreateOutput{Name: input.Name, Version: version.ID}, nil
}
