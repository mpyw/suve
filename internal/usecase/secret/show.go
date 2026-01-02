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
	secretapi.ListSecretVersionIdsAPI
}

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *secretversion.Spec
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name         string
	Value        string
	VersionID    string
	VersionStage []string
	CreatedDate  *time.Time
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

	return &ShowOutput{
		Name:         lo.FromPtr(secret.Name),
		Value:        lo.FromPtr(secret.SecretString),
		VersionID:    lo.FromPtr(secret.VersionId),
		VersionStage: secret.VersionStages,
		CreatedDate:  secret.CreatedDate,
	}, nil
}
