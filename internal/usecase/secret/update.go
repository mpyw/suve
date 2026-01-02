package secret

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/tagging"
)

// UpdateClient is the interface for the update use case.
type UpdateClient interface {
	secretapi.GetSecretValueAPI
	secretapi.UpdateSecretAPI
	secretapi.PutSecretValueAPI
	secretapi.TagResourceAPI
	secretapi.UntagResourceAPI
}

// UpdateInput holds input for the update use case.
type UpdateInput struct {
	Name        string
	Value       string
	Description string
	TagChange   *tagging.Change
}

// UpdateOutput holds the result of the update use case.
type UpdateOutput struct {
	Name      string
	VersionID string
	ARN       string
}

// UpdateUseCase executes update operations.
type UpdateUseCase struct {
	Client UpdateClient
}

// GetCurrentValue fetches the current secret value.
func (u *UpdateUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	out, err := u.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
		SecretId: lo.ToPtr(name),
	})
	if err != nil {
		return "", err
	}
	return lo.FromPtr(out.SecretString), nil
}

// Execute runs the update use case.
func (u *UpdateUseCase) Execute(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	var versionID, arn string

	// Update value
	if input.Value != "" {
		result, err := u.Client.PutSecretValue(ctx, &secretapi.PutSecretValueInput{
			SecretId:     lo.ToPtr(input.Name),
			SecretString: lo.ToPtr(input.Value),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update secret value: %w", err)
		}
		versionID = lo.FromPtr(result.VersionId)
		arn = lo.FromPtr(result.ARN)
	}

	// Update description if provided
	if input.Description != "" {
		result, err := u.Client.UpdateSecret(ctx, &secretapi.UpdateSecretInput{
			SecretId:    lo.ToPtr(input.Name),
			Description: lo.ToPtr(input.Description),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update secret description: %w", err)
		}
		if versionID == "" {
			versionID = lo.FromPtr(result.VersionId)
		}
		arn = lo.FromPtr(result.ARN)
	}

	// Handle tagging
	if input.TagChange != nil && !input.TagChange.IsEmpty() {
		// Get ARN if we don't have it yet
		if arn == "" {
			out, err := u.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
				SecretId: lo.ToPtr(input.Name),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get secret ARN: %w", err)
			}
			arn = lo.FromPtr(out.ARN)
		}

		// Add tags
		if len(input.TagChange.Add) > 0 {
			tags := make([]secretapi.Tag, 0, len(input.TagChange.Add))
			for k, v := range input.TagChange.Add {
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
				return nil, fmt.Errorf("failed to add tags: %w", err)
			}
		}

		// Remove tags
		if len(input.TagChange.Remove) > 0 {
			_, err := u.Client.UntagResource(ctx, &secretapi.UntagResourceInput{
				SecretId: lo.ToPtr(arn),
				TagKeys:  input.TagChange.Remove,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to remove tags: %w", err)
			}
		}
	}

	return &UpdateOutput{
		Name:      input.Name,
		VersionID: versionID,
		ARN:       arn,
	}, nil
}
