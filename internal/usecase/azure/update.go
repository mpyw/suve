package azure

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// UpdateInput holds input for the update use case.
type UpdateInput struct {
	Name      string
	Value     string
	ValueType domain.ValueType // secret (Key Vault) or plaintext (App Configuration)
}

// UpdateOutput holds the result of the update use case.
type UpdateOutput struct {
	Name    string
	Version string // opaque version id (Key Vault), or "" (App Configuration)
}

// UpdateUseCase executes update operations.
type UpdateUseCase struct {
	Store provider.Store
}

// GetCurrentValue fetches the current value for preview. A non-existent entry
// yields an empty value with no error; any other read failure is propagated.
func (u *UpdateUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	entry, err := u.Store.Get(ctx, name, provider.VersionRef{})

	switch {
	case errors.Is(err, provider.ErrNotFound):
		return "", nil
	case err != nil:
		return "", err
	}

	return entry.Value, nil
}

// Execute runs the update use case. It updates an existing entry; if the entry
// doesn't exist it returns ErrEntryNotFound. A read failure other than
// not-found is propagated unchanged.
func (u *UpdateUseCase) Execute(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	_, err := u.Store.Get(ctx, input.Name, provider.VersionRef{})

	switch {
	case errors.Is(err, provider.ErrNotFound):
		return nil, fmt.Errorf("%w: %s", ErrEntryNotFound, input.Name)
	case err != nil:
		return nil, err
	}

	version, err := u.Store.Put(ctx, input.Name, input.Value, input.ValueType, "")
	if err != nil {
		return nil, fmt.Errorf("failed to update entry: %w", err)
	}

	return &UpdateOutput{Name: input.Name, Version: version.ID}, nil
}
