package gcp

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/gcpversion"
)

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *gcpversion.Spec
}

// ShowTag represents a tag (label) key-value pair.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name        string
	Value       string
	Version     string // integer version number, or "" for an unknown/latest version
	State       string // enabled/disabled/destroyed (best-effort), may be ""
	CreatedDate *time.Time
	Tags        []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Reader provider.Reader
}

// Execute runs the show use case. Integer-version and ~shift resolution are
// provided by the adapter behind provider.Reader.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	ref, err := u.Reader.Resolve(ctx, input.Spec.Name, specSuffix(input.Spec))
	if err != nil {
		return nil, err
	}

	entry, err := u.Reader.Get(ctx, input.Spec.Name, ref)
	if err != nil {
		return nil, err
	}

	return &ShowOutput{
		Name:        entry.Name,
		Value:       entry.Value,
		Version:     entry.Version.ID,
		State:       entry.Version.Label,
		CreatedDate: entry.Version.Created,
		Tags: lo.Map(entry.Tags, func(tag domain.Tag, _ int) ShowTag {
			return ShowTag{Key: tag.Key, Value: tag.Value}
		}),
	}, nil
}
