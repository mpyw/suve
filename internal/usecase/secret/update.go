package secret

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// UpdateInput holds input for the update use case.
type UpdateInput struct {
	Name        string
	Value       string
	Description string
	// Options carries provider-specific write options (e.g. AWS Secrets Manager
	// KMS key, rotation). They are passed through to the provider unchanged.
	Options []provider.WriteOption
}

// UpdateOutput holds the result of the update use case.
type UpdateOutput struct {
	Name      string
	VersionID string
}

// UpdateUseCase executes update operations.
type UpdateUseCase struct {
	Store provider.Store
}

// GetCurrentValue fetches the current secret value for preview. A non-existent
// secret yields an empty value with no error; any other read failure is
// propagated.
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

// Execute runs the update use case. It updates an existing secret (new version
// plus, when provided, description); if the secret doesn't exist it returns
// ErrSecretNotFound. A read failure other than not-found is propagated unchanged
// (never treated as "does not exist").
func (u *UpdateUseCase) Execute(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	_, err := u.Store.Get(ctx, input.Name, provider.VersionRef{})

	switch {
	case errors.Is(err, provider.ErrNotFound):
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, input.Name)
	case err != nil:
		return nil, err
	}

	version, err := u.Store.Put(ctx, input.Name, input.Value, domain.ValueTypeSecret, input.Description, input.Options...)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return &UpdateOutput{
		Name:      input.Name,
		VersionID: version.ID,
	}, nil
}
