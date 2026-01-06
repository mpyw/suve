package param

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// CreateClient is the interface for the create use case.
type CreateClient interface {
	paramapi.PutParameterAPI
}

// CreateInput holds input for the create use case.
type CreateInput struct {
	Name        string
	Value       string
	Type        paramapi.ParameterType
	Description string
}

// CreateOutput holds the result of the create use case.
type CreateOutput struct {
	Name    string
	Version int64
}

// CreateUseCase executes create operations.
type CreateUseCase struct {
	Client CreateClient
}

// Execute runs the create use case.
// It creates a new parameter. If the parameter already exists, returns an error.
func (u *CreateUseCase) Execute(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	putInput := &paramapi.PutParameterInput{
		Name:      lo.ToPtr(input.Name),
		Value:     lo.ToPtr(input.Value),
		Type:      input.Type,
		Overwrite: lo.ToPtr(false), // Do not overwrite existing parameters
	}
	if input.Description != "" {
		putInput.Description = lo.ToPtr(input.Description)
	}

	putOutput, err := u.Client.PutParameter(ctx, putInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create parameter: %w", err)
	}

	return &CreateOutput{
		Name:    input.Name,
		Version: putOutput.Version,
	}, nil
}
