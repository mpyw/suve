package param

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// SetClient is the interface for the set use case.
type SetClient interface {
	paramapi.GetParameterAPI
	paramapi.PutParameterAPI
}

// SetInput holds input for the set use case.
type SetInput struct {
	Name        string
	Value       string
	Type        paramapi.ParameterType
	Description string
}

// SetOutput holds the result of the set use case.
type SetOutput struct {
	Name      string
	Version   int64
	IsCreated bool // true if created, false if updated
}

// SetUseCase executes set operations.
type SetUseCase struct {
	Client SetClient
}

// Exists checks if a parameter exists.
func (u *SetUseCase) Exists(ctx context.Context, name string) (bool, error) {
	_, err := u.Client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		var pnf *paramapi.ParameterNotFound
		if errors.As(err, &pnf) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Execute runs the set use case.
func (u *SetUseCase) Execute(ctx context.Context, input SetInput) (*SetOutput, error) {
	// Check if parameter exists (for determining create vs update)
	exists, err := u.Exists(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	// Put parameter
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
		return nil, fmt.Errorf("failed to put parameter: %w", err)
	}

	return &SetOutput{
		Name:      input.Name,
		Version:   putOutput.Version,
		IsCreated: !exists,
	}, nil
}
