package param

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// UpdateClient is the interface for the update use case.
type UpdateClient interface {
	paramapi.GetParameterAPI
	paramapi.PutParameterAPI
}

// UpdateInput holds input for the update use case.
type UpdateInput struct {
	Name        string
	Value       string
	Type        paramapi.ParameterType
	Description string
}

// UpdateOutput holds the result of the update use case.
type UpdateOutput struct {
	Name    string
	Version int64
}

// UpdateUseCase executes update operations.
type UpdateUseCase struct {
	Client UpdateClient
}

// Exists checks if a parameter exists.
func (u *UpdateUseCase) Exists(ctx context.Context, name string) (bool, error) {
	_, err := u.Client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		if pnf := (*paramapi.ParameterNotFound)(nil); errors.As(err, &pnf) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Execute runs the update use case.
// It updates an existing parameter. If the parameter doesn't exist, returns an error.
func (u *UpdateUseCase) Execute(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	// Check if parameter exists
	exists, err := u.Exists(ctx, input.Name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("parameter not found: %s", input.Name)
	}

	// Update parameter
	putInput := &paramapi.PutParameterInput{
		Name:      lo.ToPtr(input.Name),
		Value:     lo.ToPtr(input.Value),
		Type:      input.Type,
		Overwrite: lo.ToPtr(true),
	}
	if input.Description != "" {
		putInput.Description = lo.ToPtr(input.Description)
	}

	putOutput, err := u.Client.PutParameter(ctx, putInput)
	if err != nil {
		return nil, fmt.Errorf("failed to update parameter: %w", err)
	}

	return &UpdateOutput{
		Name:    input.Name,
		Version: putOutput.Version,
	}, nil
}
