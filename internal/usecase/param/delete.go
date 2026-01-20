package param

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/model"
)

// DeleteClient is the interface for the delete use case.
type DeleteClient interface {
	// GetParameter retrieves a parameter by name and optional version.
	GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error)
	// DeleteParameter deletes a parameter by name.
	DeleteParameter(ctx context.Context, name string) error
}

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name string
}

// DeleteOutput holds the result of the delete use case.
type DeleteOutput struct {
	Name string
}

// DeleteUseCase executes delete operations.
type DeleteUseCase struct {
	Client DeleteClient
}

// GetCurrentValue fetches the current value for preview.
func (u *DeleteUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	param, err := u.Client.GetParameter(ctx, name, "")
	if err != nil {
		// Treat any error as "not found" for simplicity
		return "", nil //nolint:nilerr // intentionally ignoring error to treat as not found
	}

	return param.Value, nil
}

// Execute runs the delete use case.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	err := u.Client.DeleteParameter(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to delete parameter: %w", err)
	}

	return &DeleteOutput{Name: input.Name}, nil
}
