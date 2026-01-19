// Package secret provides use cases for Secrets Manager operations.
package secret

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/resource"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// ShowClient is the interface for the show use case.
type ShowClient interface {
	provider.SecretReader
	provider.SecretTagger
}

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *secretversion.Spec
}

// ShowTag represents a tag key-value pair.
type ShowTag = resource.ShowTag

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name         string
	ARN          string
	Value        string
	VersionID    string
	VersionStage []string
	Description  string
	CreatedDate  *time.Time
	Tags         []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Client ShowClient
}

// Execute runs the show use case.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	// Resolve secret version
	secret, err := secretversion.Resolve(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	// Use unified resource usecase for common logic (tag fetching)
	uc := &resource.ShowUseCase{Client: u.Client}

	result, err := uc.Execute(ctx, resource.ShowInput{
		Resource: secret.ToResource(),
	})
	if err != nil {
		return nil, err
	}

	// Convert to secret-specific output
	output := &ShowOutput{
		Name:        result.Name,
		ARN:         result.ARN,
		Value:       result.Value,
		VersionID:   result.Version,
		Description: result.Description,
		CreatedDate: result.ModifiedAt,
		Tags:        result.Tags,
	}

	// Extract version stages from metadata if available
	if meta, ok := result.Metadata.(model.AWSSecretMeta); ok {
		output.VersionStage = meta.VersionStages
	}

	return output, nil
}
