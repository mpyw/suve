// Package secret provides use cases for Secrets Manager operations.
package secret

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
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
type ShowTag struct {
	Key   string
	Value string
}

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
	secret, err := secretversion.Resolve(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:        secret.Name,
		ARN:         secret.ARN,
		Value:       secret.Value,
		VersionID:   secret.VersionID,
		Description: secret.Description,
		CreatedDate: secret.CreatedDate,
	}

	// Extract version stages from metadata if available
	if meta, ok := secret.Metadata.(model.AWSSecretMeta); ok {
		output.VersionStage = meta.VersionStages
	}

	// Fetch tags
	tags, err := u.Client.GetTags(ctx, secret.Name)
	if err == nil && tags != nil {
		output.Tags = make([]ShowTag, 0, len(tags))
		for k, v := range tags {
			output.Tags = append(output.Tags, ShowTag{
				Key:   k,
				Value: v,
			})
		}
	}

	return output, nil
}
