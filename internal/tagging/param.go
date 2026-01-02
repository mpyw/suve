package tagging

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// ParamClient is the interface for SSM Parameter Store tag operations.
type ParamClient interface {
	AddTagsToResource(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error)
}

// ApplyParam applies tag changes to an SSM Parameter Store parameter.
// The resourceID should be the parameter name (e.g., "/my/param").
func ApplyParam(ctx context.Context, client ParamClient, resourceID string, change *Change) error {
	if change.IsEmpty() {
		return nil
	}

	// Add tags
	if len(change.Add) > 0 {
		tags := make([]paramapi.Tag, 0, len(change.Add))
		for k, v := range change.Add {
			tags = append(tags, paramapi.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
		_, err := client.AddTagsToResource(ctx, &paramapi.AddTagsToResourceInput{
			ResourceType: paramapi.ResourceTypeForTaggingParameter,
			ResourceId:   lo.ToPtr(resourceID),
			Tags:         tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if len(change.Remove) > 0 {
		_, err := client.RemoveTagsFromResource(ctx, &paramapi.RemoveTagsFromResourceInput{
			ResourceType: paramapi.ResourceTypeForTaggingParameter,
			ResourceId:   lo.ToPtr(resourceID),
			TagKeys:      change.Remove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
