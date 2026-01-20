package param

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mpyw/suve/internal/model"
)

// CreateClient is the interface for the create use case.
type CreateClient interface {
	// PutParameter creates or updates a parameter.
	PutParameter(ctx context.Context, param *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error)
}

// CreateInput holds input for the create use case.
type CreateInput struct {
	Name        string
	Value       string
	Type        string // Parameter type (e.g., "String", "SecureString")
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
	param := &model.Parameter{
		Name:        input.Name,
		Value:       input.Value,
		Description: input.Description,
		Metadata:    model.AWSParameterMeta{Type: input.Type},
	}

	result, err := u.Client.PutParameter(ctx, param, false) // Do not overwrite existing parameters
	if err != nil {
		return nil, fmt.Errorf("failed to create parameter: %w", err)
	}

	version, _ := strconv.ParseInt(result.Version, 10, 64)

	return &CreateOutput{
		Name:    result.Name,
		Version: version,
	}, nil
}
