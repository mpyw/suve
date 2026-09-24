// Package param provides the provider-neutral use cases of the param service
// axis (AWS Systems Manager Parameter Store, Azure App Configuration).
//
// The use cases speak only the neutral provider seam: an entry is addressed by
// its name plus an opaque version suffix (e.g. "#3", "~2", or "" for the latest
// version) that the caller rebuilt with its provider's grammar and that the
// adapter re-parses in Reader.Resolve. Versions come back as opaque ids;
// provider-specific rendering (e.g. AWS integer versions) belongs to the
// presenters.
package param

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// ShowInput holds input for the show use case.
type ShowInput struct {
	Name string
	// Suffix is the version-spec suffix after the name ("#3", "~2", or "" for
	// the latest version), re-parsed by the adapter.
	Suffix string
}

// ShowTag represents a tag key-value pair.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name         string
	Value        string
	Version      string // opaque version id
	Type         domain.ValueType
	Description  string
	LastModified *time.Time
	Tags         []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Reader provider.Reader
}

// Execute runs the show use case.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	ref, err := u.Reader.Resolve(ctx, input.Name, input.Suffix)
	if err != nil {
		return nil, err
	}

	entry, err := u.Reader.Get(ctx, input.Name, ref)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:         entry.Name,
		Value:        entry.Value,
		Version:      entry.Version.ID,
		Type:         entry.Type,
		Description:  entry.Description,
		LastModified: entry.Modified,
		Tags: lo.Map(entry.Tags, func(tag domain.Tag, _ int) ShowTag {
			return ShowTag{Key: tag.Key, Value: tag.Value}
		}),
	}

	return output, nil
}
