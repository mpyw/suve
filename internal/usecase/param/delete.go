package param

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// DeleteClient is the interface for the delete use case.
type DeleteClient interface {
	paramapi.DeleteParameterAPI
	paramapi.GetParameterAPI
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
	out, err := u.Client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name:           lo.ToPtr(name),
		WithDecryption: lo.ToPtr(true),
	})
	if err != nil {
		pnf := (*paramapi.ParameterNotFound)(nil)
		if errors.As(err, &pnf) {
			return "", nil
		}

		return "", err
	}

	return lo.FromPtr(out.Parameter.Value), nil
}

// Execute runs the delete use case.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	_, err := u.Client.DeleteParameter(ctx, &paramapi.DeleteParameterInput{
		Name: lo.ToPtr(input.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete parameter: %w", err)
	}

	return &DeleteOutput{Name: input.Name}, nil
}
