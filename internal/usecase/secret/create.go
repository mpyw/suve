package secret

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// CreateClient is the interface for the create use case.
type CreateClient interface {
	secretapi.CreateSecretAPI
}

// CreateInput holds input for the create use case.
type CreateInput struct {
	Name        string
	Value       string
	Description string
	Tags        map[string]string
}

// CreateOutput holds the result of the create use case.
type CreateOutput struct {
	Name      string
	VersionID string
	ARN       string
}

// CreateUseCase executes create operations.
type CreateUseCase struct {
	Client CreateClient
}

// Execute runs the create use case.
func (u *CreateUseCase) Execute(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	createInput := &secretapi.CreateSecretInput{
		Name:         lo.ToPtr(input.Name),
		SecretString: lo.ToPtr(input.Value),
	}
	if input.Description != "" {
		createInput.Description = lo.ToPtr(input.Description)
	}
	if len(input.Tags) > 0 {
		tags := make([]secretapi.Tag, 0, len(input.Tags))
		for k, v := range input.Tags {
			tags = append(tags, secretapi.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
		createInput.Tags = tags
	}

	result, err := u.Client.CreateSecret(ctx, createInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return &CreateOutput{
		Name:      lo.FromPtr(result.Name),
		VersionID: lo.FromPtr(result.VersionId),
		ARN:       lo.FromPtr(result.ARN),
	}, nil
}
