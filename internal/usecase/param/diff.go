package param

import (
	"context"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// DiffClient is the interface for the diff use case.
type DiffClient interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
}

// DiffInput holds input for the diff use case.
type DiffInput struct {
	Spec1 *paramversion.Spec
	Spec2 *paramversion.Spec
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	OldName    string
	OldVersion int64
	OldValue   string
	NewName    string
	NewVersion int64
	NewValue   string
}

// DiffUseCase executes diff operations.
type DiffUseCase struct {
	Client DiffClient
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	param1, err := paramversion.GetParameterWithVersion(ctx, u.Client, input.Spec1, true)
	if err != nil {
		return nil, err
	}

	param2, err := paramversion.GetParameterWithVersion(ctx, u.Client, input.Spec2, true)
	if err != nil {
		return nil, err
	}

	return &DiffOutput{
		OldName:    lo.FromPtr(param1.Name),
		OldVersion: param1.Version,
		OldValue:   lo.FromPtr(param1.Value),
		NewName:    lo.FromPtr(param2.Name),
		NewVersion: param2.Version,
		NewValue:   lo.FromPtr(param2.Value),
	}, nil
}
