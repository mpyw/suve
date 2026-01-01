package tagging

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
)

// SSMClient is the interface for SSM tag operations.
type SSMClient interface {
	AddTagsToResource(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error)
}

// ApplySSM applies tag changes to an SSM parameter.
// The resourceID should be the parameter name (e.g., "/my/param").
func ApplySSM(ctx context.Context, client SSMClient, resourceID string, change *Change) error {
	if change.IsEmpty() {
		return nil
	}

	// Add tags
	if len(change.Add) > 0 {
		tags := make([]types.Tag, 0, len(change.Add))
		for k, v := range change.Add {
			tags = append(tags, types.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
		_, err := client.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
			ResourceType: types.ResourceTypeForTaggingParameter,
			ResourceId:   lo.ToPtr(resourceID),
			Tags:         tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if len(change.Remove) > 0 {
		_, err := client.RemoveTagsFromResource(ctx, &ssm.RemoveTagsFromResourceInput{
			ResourceType: types.ResourceTypeForTaggingParameter,
			ResourceId:   lo.ToPtr(resourceID),
			TagKeys:      change.Remove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
