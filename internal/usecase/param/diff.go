package param

import (
	"context"
	"strconv"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// DiffClient is the interface for the diff use case.
//
//nolint:iface // Intentionally aliases ParameterReader for type clarity in DiffUseCase.
type DiffClient interface {
	provider.ParameterReader
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
	param1, err := paramversion.GetParameterWithVersion(ctx, u.Client, input.Spec1)
	if err != nil {
		return nil, err
	}

	param2, err := paramversion.GetParameterWithVersion(ctx, u.Client, input.Spec2)
	if err != nil {
		return nil, err
	}

	oldVersion, _ := strconv.ParseInt(param1.Version, 10, 64)
	newVersion, _ := strconv.ParseInt(param2.Version, 10, 64)

	return &DiffOutput{
		OldName:    param1.Name,
		OldVersion: oldVersion,
		OldValue:   param1.Value,
		NewName:    param2.Name,
		NewVersion: newVersion,
		NewValue:   param2.Value,
	}, nil
}
