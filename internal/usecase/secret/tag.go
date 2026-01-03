package secret

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// TagClient is the interface for the tag use case.
type TagClient interface {
	secretapi.DescribeSecretAPI
	secretapi.TagResourceAPI
	secretapi.UntagResourceAPI
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
	// Get ARN first (required for tagging)
	desc, err := u.Client.DescribeSecret(ctx, &secretapi.DescribeSecretInput{
		SecretId: lo.ToPtr(input.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to describe secret: %w", err)
	}
	arn := lo.FromPtr(desc.ARN)

	// Add tags
	if len(input.Add) > 0 {
		tags := make([]secretapi.Tag, 0, len(input.Add))
		for k, v := range input.Add {
			tags = append(tags, secretapi.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
		_, err := u.Client.TagResource(ctx, &secretapi.TagResourceInput{
			SecretId: lo.ToPtr(arn),
			Tags:     tags,
		})
		if err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if len(input.Remove) > 0 {
		_, err := u.Client.UntagResource(ctx, &secretapi.UntagResourceInput{
			SecretId: lo.ToPtr(arn),
			TagKeys:  input.Remove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
