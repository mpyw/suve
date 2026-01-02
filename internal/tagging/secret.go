package tagging

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
)

// SecretClient is the interface for Secrets Manager tag operations.
type SecretClient interface {
	TagResource(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error)
	UntagResource(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error)
}

// ApplySecret applies tag changes to a Secrets Manager secret.
// The secretID should be the secret name or ARN.
func ApplySecret(ctx context.Context, client SecretClient, secretID string, change *Change) error {
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
		_, err := client.TagResource(ctx, &secretsmanager.TagResourceInput{
			SecretId: lo.ToPtr(secretID),
			Tags:     tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if len(change.Remove) > 0 {
		_, err := client.UntagResource(ctx, &secretsmanager.UntagResourceInput{
			SecretId: lo.ToPtr(secretID),
			TagKeys:  change.Remove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
