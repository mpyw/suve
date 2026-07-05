package azure

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// CreateInput holds input for the create use case.
type CreateInput struct {
	Name      string
	Value     string
	ValueType domain.ValueType // secret (Key Vault) or plaintext (App Configuration)
}

// CreateOutput holds the result of the create use case.
type CreateOutput struct {
	Name    string
	Version string // opaque version id (Key Vault), or "" (App Configuration)
}

// CreateUseCase executes create operations.
type CreateUseCase struct {
	Writer provider.Writer
}

// Execute runs the create use case. It creates a new entry via the provider; if
// the entry already exists the provider returns a wrapped
// provider.ErrAlreadyExists and no overwrite occurs.
func (u *CreateUseCase) Execute(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	version, err := u.Writer.Create(ctx, input.Name, input.Value, input.ValueType, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create entry: %w", err)
	}

	return &CreateOutput{Name: input.Name, Version: version.ID}, nil
}
