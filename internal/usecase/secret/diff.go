package secret

import (
	"context"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// DiffInput holds input for the diff use case. Each side is a name plus its
// version-spec suffix (see ShowInput.Suffix).
type DiffInput struct {
	Name1   string
	Suffix1 string
	Name2   string
	Suffix2 string
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	OldName    string
	OldVersion string // opaque version id
	OldValue   string
	NewName    string
	NewVersion string // opaque version id
	NewValue   string
}

// DiffUseCase executes diff operations.
type DiffUseCase struct {
	Reader provider.Reader
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	entry1, err := u.resolveAndGet(ctx, input.Name1, input.Suffix1)
	if err != nil {
		return nil, err
	}

	entry2, err := u.resolveAndGet(ctx, input.Name2, input.Suffix2)
	if err != nil {
		return nil, err
	}

	return &DiffOutput{
		OldName:    entry1.Name,
		OldVersion: entry1.Version.ID,
		OldValue:   entry1.Value,
		NewName:    entry2.Name,
		NewVersion: entry2.Version.ID,
		NewValue:   entry2.Value,
	}, nil
}

// resolveAndGet resolves a name+suffix to a version ref and fetches the entry.
func (u *DiffUseCase) resolveAndGet(ctx context.Context, name, suffix string) (*domain.Entry, error) {
	ref, err := u.Reader.Resolve(ctx, name, suffix)
	if err != nil {
		return nil, err
	}

	return u.Reader.Get(ctx, name, ref)
}
