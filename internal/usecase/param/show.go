// Package param provides use cases for SSM Parameter Store operations.
package param

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/provider"
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
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name        string
	Value       string
	Version     string
	Type        string // Parameter type (e.g., "String", "SecureString")
	Description string
	UpdatedAt   *time.Time
	Tags        []ShowTag
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
		Name:        param.Name,
		Value:       param.Value,
		Version:     param.Version,
		Description: param.Description,
		UpdatedAt:   param.UpdatedAt,
	}

	// Extract Type from AWS metadata if available
	if meta := param.AWSMeta(); meta != nil {
		output.Type = meta.Type
	}

	// Fetch tags
	tags, err := u.Client.GetTags(ctx, param.Name)
	if err == nil && tags != nil {
		for k, v := range tags {
			output.Tags = append(output.Tags, ShowTag{Key: k, Value: v})
		}
	}

	return output, nil
}
