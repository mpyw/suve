// Package secret provides use cases for Secrets Manager operations.
package secret

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// ShowClient is the interface for the show use case.
type ShowClient interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIDsAPI
	secretapi.DescribeSecretAPI
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
	secret, err := secretversion.GetSecretWithVersion(ctx, u.Client, input.Spec)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:         lo.FromPtr(secret.Name),
		ARN:          lo.FromPtr(secret.ARN),
		Value:        lo.FromPtr(secret.SecretString),
		VersionID:    lo.FromPtr(secret.VersionId),
		VersionStage: secret.VersionStages,
		CreatedDate:  secret.CreatedDate,
	}

	// Fetch tags and description via DescribeSecret
	describeOutput, err := u.Client.DescribeSecret(ctx, &secretapi.DescribeSecretInput{
		SecretId: secret.Name,
	})
	if err == nil && describeOutput != nil {
		output.Description = lo.FromPtr(describeOutput.Description)
		for _, tag := range describeOutput.Tags {
			output.Tags = append(output.Tags, ShowTag{
				Key:   lo.FromPtr(tag.Key),
				Value: lo.FromPtr(tag.Value),
			})
		}
	}

	return output, nil
}
