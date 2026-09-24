package param

import (
	"context"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// DiffInput holds input for the diff use case. Each side is a name plus its
// version-spec suffix (see ShowInput.Suffix), so an unversioned store can
// compare two distinct names.
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
	// Secret reports whether either version holds a secret value (e.g. an AWS
	// SecureString), so a consumer masks both sides before rendering the diff. A
	// secret-typed param is a secret on the value-type axis even though it lives
	// on the param service axis, so masking must key off this flag, not the
	// service (#677/#702).
	Secret bool
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
		Secret:     entry1.Type == domain.ValueTypeSecret || entry2.Type == domain.ValueTypeSecret,
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
