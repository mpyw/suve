// Package param provides use cases for SSM Parameter Store operations.
package param

import (
	"context"
	"strconv"
	"time"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/resource"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// ShowClient is the interface for the show use case.
type ShowClient interface {
	provider.ParameterReader
	provider.ParameterTagger
}

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *paramversion.Spec
}

// ShowTag represents a tag key-value pair.
type ShowTag = resource.ShowTag

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name         string
	Value        string
	Version      int64
	Type         string
	Description  string
	LastModified *time.Time
	Tags         []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Client ShowClient
}

// Execute runs the show use case.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	// Resolve parameter version
	param, err := paramversion.Resolve(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	// Use unified resource usecase for common logic (tag fetching)
	uc := &resource.ShowUseCase{Client: u.Client}

	result, err := uc.Execute(ctx, resource.ShowInput{
		Resource: param.ToResource(),
	})
	if err != nil {
		return nil, err
	}

	// Convert to param-specific output
	version, _ := strconv.ParseInt(result.Version, 10, 64)

	return &ShowOutput{
		Name:         result.Name,
		Value:        result.Value,
		Version:      version,
		Type:         result.Type,
		Description:  result.Description,
		LastModified: result.ModifiedAt,
		Tags:         result.Tags,
	}, nil
}
