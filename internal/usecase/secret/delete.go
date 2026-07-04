package secret

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
)

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name string
	// Options carries provider-specific delete options (e.g. AWS Secrets Manager
	// ForceDelete / RecoveryWindow). They are passed through to the provider
	// unchanged.
	Options []provider.DeleteOption
}

// DeleteOutput holds the result of the delete use case.
type DeleteOutput struct {
	Name string
}

// DeleteUseCase executes delete operations.
type DeleteUseCase struct {
	Store provider.Store
}

// GetCurrentValue fetches the current secret value for preview. A non-existent
// secret yields an empty value with no error; any other read failure is
// propagated.
func (u *DeleteUseCase) GetCurrentValue(ctx context.Context, name string) (string, error) {
	entry, err := u.Store.Get(ctx, name, provider.VersionRef{})

	switch {
	case errors.Is(err, provider.ErrNotFound):
		return "", nil
	case err != nil:
		return "", err
	}

	return entry.Value, nil
}

// Execute runs the delete use case. Deletion behavior (force / recovery window)
// is carried by the provider.DeleteOptions in the input.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if err := u.Store.Delete(ctx, input.Name, input.Options...); err != nil {
		return nil, fmt.Errorf("failed to delete secret: %w", err)
	}

	return &DeleteOutput{Name: input.Name}, nil
}
