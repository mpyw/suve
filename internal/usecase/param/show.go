// Package param provides use cases for SSM Parameter Store operations.
package param

import (
	"context"
	"strconv"
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
	param, err := paramversion.Resolve(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	version, _ := strconv.ParseInt(param.Version, 10, 64)

	output := &ShowOutput{
		Name:         param.Name,
		Value:        param.Value,
		Version:      version,
		Type:         param.Type,
		Description:  param.Description,
		LastModified: param.LastModified,
	}

	// Fetch tags
	tags, err := u.Client.GetTags(ctx, param.Name)
	if err == nil && tags != nil {
		output.Tags = make([]ShowTag, 0, len(tags))
		for k, v := range tags {
			output.Tags = append(output.Tags, ShowTag{
				Key:   k,
				Value: v,
			})
		}
	}

	return output, nil
}
