package param

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
	Type        domain.ValueType
	Description string
	// PreserveType, when true, keeps the existing parameter's type instead of
	// applying Type, so a value-only update never downgrades an existing secret
	// or list value (e.g. an AWS SecureString losing its KMS encryption) to
	// plaintext. The AWS CLI sets it when no type flag is given; callers that
	// pass an explicit type (GUI, TUI) leave it false and Type is applied as-is.
	PreserveType bool
	// Options carries provider-specific write options (e.g. AWS param Tier,
	// DataType). They are passed through to the provider unchanged.
	Options []provider.WriteOption
}

// UpdateOutput holds the result of the update use case.
type UpdateOutput struct {
	Name    string
	Version string // opaque version id; "" for an unversioned store
}

// UpdateUseCase executes update operations.
type UpdateUseCase struct {
	Store provider.Store
	// ItemNoun names one item of the service in error messages ("parameter",
	// "setting"; capability.ServiceCapability.ItemNoun). Empty means "entry".
	ItemNoun string
}

// GetCurrentValue fetches the current parameter value for preview. A
// non-existent parameter yields an empty value with no error; any other read
// failure is propagated.
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

// Execute runs the update use case. It updates an existing parameter; if the
// parameter doesn't exist it returns a wrapped ErrNotFound. A read failure other
// than not-found is propagated unchanged (never treated as "does not exist").
func (u *UpdateUseCase) Execute(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	entry, err := u.Store.Get(ctx, input.Name, provider.VersionRef{})

	switch {
	case errors.Is(err, provider.ErrNotFound):
		return nil, fmt.Errorf("%s %w: %s", errItemNoun(u.ItemNoun), ErrNotFound, input.Name)
	case err != nil:
		return nil, err
	}

	// A value-only update must not change the type. When the caller did not
	// specify one, reuse the existing entry's type (already fetched above) so a
	// secret/list value is never silently rewritten as plaintext.
	valueType := input.Type
	if input.PreserveType {
		valueType = entry.Type
	}

	version, err := u.Store.Put(ctx, input.Name, input.Value, valueType, input.Description, input.Options...)
	if err != nil {
		return nil, fmt.Errorf("failed to update %s: %w", errItemNoun(u.ItemNoun), err)
	}

	return &UpdateOutput{
		Name:    input.Name,
		Version: version.ID,
	}, nil
}
