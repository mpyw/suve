package secret

import (
	"context"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/version/secretversion"
)

// DiffClient is the interface for the diff use case.
type DiffClient = VersionResolverClient

// DiffInput holds input for the diff use case.
type DiffInput struct {
	Spec1 *secretversion.Spec
	Spec2 *secretversion.Spec
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	OldName      string
	OldVersionID string
	OldValue     string
	NewName      string
	NewVersionID string
	NewValue     string
}

// DiffUseCase executes diff operations.
type DiffUseCase struct {
	Client DiffClient
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	secret1, err := secretversion.GetSecretWithVersion(ctx, u.Client, input.Spec1)
	if err != nil {
		return nil, err
	}

	secret2, err := secretversion.GetSecretWithVersion(ctx, u.Client, input.Spec2)
	if err != nil {
		return nil, err
	}

	return &DiffOutput{
		OldName:      lo.FromPtr(secret1.Name),
		OldVersionID: lo.FromPtr(secret1.VersionId),
		OldValue:     lo.FromPtr(secret1.SecretString),
		NewName:      lo.FromPtr(secret2.Name),
		NewVersionID: lo.FromPtr(secret2.VersionId),
		NewValue:     lo.FromPtr(secret2.SecretString),
	}, nil
}
