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
	paramapi.ListTagsForResourceAPI
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
	Type         paramapi.ParameterType
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
	param, err := paramversion.GetParameterWithVersion(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:        lo.FromPtr(param.Name),
		Value:       lo.FromPtr(param.Value),
		Version:     param.Version,
		Type:        param.Type,
		Description: lo.FromPtr(param.Description),
	}
	if param.LastModifiedDate != nil {
		output.LastModified = param.LastModifiedDate
	}

	// Fetch tags
	tagsOutput, err := u.Client.ListTagsForResource(ctx, &paramapi.ListTagsForResourceInput{
		ResourceType: paramapi.ResourceTypeForTaggingParameter,
		ResourceId:   param.Name,
	})
	if err == nil && tagsOutput != nil {
		output.Tags = lo.Map(tagsOutput.TagList, func(tag paramapi.Tag, _ int) ShowTag {
			return ShowTag{
				Key:   lo.FromPtr(tag.Key),
				Value: lo.FromPtr(tag.Value),
			}
		})
	}

	return output, nil
}
