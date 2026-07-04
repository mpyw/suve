package param

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
	Type        domain.ValueType
	Description string
}

// CreateOutput holds the result of the create use case.
type CreateOutput struct {
	Name    string
	Version int64
}

// CreateUseCase executes create operations.
type CreateUseCase struct {
	Writer provider.Writer
}

// Execute runs the create use case. It creates a new parameter via the
// provider; if the parameter already exists the provider returns a wrapped
// provider.ErrAlreadyExists and no overwrite occurs.
func (u *CreateUseCase) Execute(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	version, err := u.Writer.Create(ctx, input.Name, input.Value, input.Type, input.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to create parameter: %w", err)
	}

	return &CreateOutput{
		Name:    input.Name,
		Version: parseVersion(version.ID),
	}, nil
}
