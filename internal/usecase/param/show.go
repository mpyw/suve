// Package param provides use cases for SSM Parameter Store operations.
package param

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// ShowClient is the interface for the show use case.
type ShowClient interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
}

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *paramversion.Spec
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name         string
	Value        string
	Version      int64
	Type         paramapi.ParameterType
	LastModified *time.Time
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Client ShowClient
}

// Execute runs the show use case.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	param, err := paramversion.GetParameterWithVersion(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:    lo.FromPtr(param.Name),
		Value:   lo.FromPtr(param.Value),
		Version: param.Version,
		Type:    param.Type,
	}
	if param.LastModifiedDate != nil {
		output.LastModified = param.LastModifiedDate
	}

	return output, nil
}
