package tagging

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// SecretClient is the interface for Secrets Manager tag operations.
type SecretClient interface {
	TagResource(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error)
	UntagResource(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error)
}

// ApplySecret applies tag changes to a Secrets Manager secret.
// The secretID should be the secret name or ARN.
func ApplySecret(ctx context.Context, client SecretClient, secretID string, change *Change) error {
	if change.IsEmpty() {
		return nil
	}

	// Add tags
	if len(change.Add) > 0 {
		tags := make([]secretapi.Tag, 0, len(change.Add))
		for k, v := range change.Add {
			tags = append(tags, secretapi.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
		_, err := client.TagResource(ctx, &secretapi.TagResourceInput{
			SecretId: lo.ToPtr(secretID),
			Tags:     tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if change.Remove.Len() > 0 {
		_, err := client.UntagResource(ctx, &secretapi.UntagResourceInput{
			SecretId: lo.ToPtr(secretID),
			TagKeys:  change.Remove.Values(),
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
