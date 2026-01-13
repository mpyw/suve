package param

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// TagClient is the interface for the tag use case.
type TagClient interface {
	paramapi.AddTagsToResourceAPI
	paramapi.RemoveTagsFromResourceAPI
}

// TagInput holds input for the tag use case.
type TagInput struct {
	Name   string
	Add    map[string]string // Tags to add or update
	Remove []string          // Tag keys to remove
}

// TagUseCase executes tag operations.
type TagUseCase struct {
	Client TagClient
}

// Execute runs the tag use case.
func (u *TagUseCase) Execute(ctx context.Context, input TagInput) error {
	// Add tags
	if len(input.Add) > 0 {
		tags := lo.MapToSlice(input.Add, func(k, v string) paramapi.Tag {
			return paramapi.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			}
		})

		_, err := u.Client.AddTagsToResource(ctx, &paramapi.AddTagsToResourceInput{
			ResourceId:   lo.ToPtr(input.Name),
			ResourceType: paramapi.ResourceTypeForTaggingParameter,
			Tags:         tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if len(input.Remove) > 0 {
		_, err := u.Client.RemoveTagsFromResource(ctx, &paramapi.RemoveTagsFromResourceInput{
			ResourceId:   lo.ToPtr(input.Name),
			ResourceType: paramapi.ResourceTypeForTaggingParameter,
			TagKeys:      input.Remove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
