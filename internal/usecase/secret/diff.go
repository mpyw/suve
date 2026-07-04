package secret

import (
	"context"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/secretversion"
)

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
	Reader provider.Reader
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	entry1, err := u.resolveAndGet(ctx, input.Spec1)
	if err != nil {
		return nil, err
	}

	entry2, err := u.resolveAndGet(ctx, input.Spec2)
	if err != nil {
		return nil, err
	}

	return &DiffOutput{
		OldName:      entry1.Name,
		OldVersionID: entry1.Version.ID,
		OldValue:     entry1.Value,
		NewName:      entry2.Name,
		NewVersionID: entry2.Version.ID,
		NewValue:     entry2.Value,
	}, nil
}

// resolveAndGet resolves a spec to a version ref and fetches the entry.
func (u *DiffUseCase) resolveAndGet(ctx context.Context, spec *secretversion.Spec) (*domain.Entry, error) {
	ref, err := u.Reader.Resolve(ctx, spec.Name, specSuffix(spec))
	if err != nil {
		return nil, err
	}

	return u.Reader.Get(ctx, spec.Name, ref)
}
