package azure

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
)

// RestoreInput holds input for the restore use case.
type RestoreInput struct {
	Name string
}

// RestoreOutput holds the result of the restore use case.
type RestoreOutput struct {
	Name string
}

// RestoreUseCase recovers a soft-deleted entry via a provider.Restorer (Azure
// Key Vault's RecoverDeletedSecret).
type RestoreUseCase struct {
	Restorer provider.Restorer
}

// Execute runs the restore use case.
func (u *RestoreUseCase) Execute(ctx context.Context, input RestoreInput) (*RestoreOutput, error) {
	if err := u.Restorer.Restore(ctx, input.Name); err != nil {
		return nil, fmt.Errorf("failed to restore entry: %w", err)
	}

	return &RestoreOutput{Name: input.Name}, nil
}
