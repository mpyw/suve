package param

import (
	"context"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/awsparamversion"
)

// DiffInput holds input for the diff use case.
type DiffInput struct {
	Spec1 *awsparamversion.Spec
	Spec2 *awsparamversion.Spec
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	OldName    string
	OldVersion int64
	OldValue   string
	NewName    string
	NewVersion int64
	NewValue   string
	// Secret reports whether either version is a SecureString (secret) value, so
	// a consumer masks both sides before rendering the diff. A SecureString param
	// is a secret on the value-type axis even though it lives on the param service
	// axis, so masking must key off this flag, not the service (#677/#702).
	Secret bool
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
		OldName:    entry1.Name,
		OldVersion: parseVersion(entry1.Version.ID),
		OldValue:   entry1.Value,
		NewName:    entry2.Name,
		NewVersion: parseVersion(entry2.Version.ID),
		NewValue:   entry2.Value,
		Secret:     entry1.Type == domain.ValueTypeSecret || entry2.Type == domain.ValueTypeSecret,
	}, nil
}

// resolveAndGet resolves a spec to a version ref and fetches the entry.
func (u *DiffUseCase) resolveAndGet(ctx context.Context, spec *awsparamversion.Spec) (*domain.Entry, error) {
	ref, err := u.Reader.Resolve(ctx, spec.Name, specSuffix(spec))
	if err != nil {
		return nil, err
	}

	return u.Reader.Get(ctx, spec.Name, ref)
}
