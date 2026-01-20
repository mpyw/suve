package param

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mpyw/suve/internal/model"
)

// UpdateClient is the interface for the update use case.
type UpdateClient interface {
	// GetParameter retrieves a parameter by name and optional version.
	GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error)
	// PutParameter creates or updates a parameter.
	PutParameter(ctx context.Context, param *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error)
}

// UpdateInput holds input for the update use case.
type UpdateInput struct {
	Name        string
	Value       string
	Type        string // Parameter type (e.g., "String", "SecureString")
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
	_, err := u.Client.GetParameter(ctx, name, "")
	if err != nil {
		// Check if it's a "not found" error
		// The error message from AWS adapter contains "failed to get parameter"
		// For now, we treat any error as "not found" for simplicity
		// A more robust solution would be to define error types in provider package
		return false, nil //nolint:nilerr // intentionally ignoring error to treat as not found
	}

	return true, nil
}

// GetCurrentValue fetches the current parameter value.
func (u *UpdateUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	param, err := u.Client.GetParameter(ctx, name, "")
	if err != nil {
		// Treat any error as "not found" for simplicity
		return "", nil //nolint:nilerr // intentionally ignoring error to treat as not found
	}

	return param.Value, nil
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
		return nil, fmt.Errorf("%w: %s", ErrParameterNotFound, input.Name)
	}

	// Update parameter
	param := &model.Parameter{
		Name:        input.Name,
		Value:       input.Value,
		Description: input.Description,
		Metadata:    model.AWSParameterMeta{Type: input.Type},
	}

	result, err := u.Client.PutParameter(ctx, param, true) // Overwrite existing
	if err != nil {
		return nil, fmt.Errorf("failed to update parameter: %w", err)
	}

	version, _ := strconv.ParseInt(result.Version, 10, 64)

	return &UpdateOutput{
		Name:    result.Name,
		Version: version,
	}, nil
}
