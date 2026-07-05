package azure

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// ShowInput holds input for the show use case.
type ShowInput struct {
	Name   string
	Suffix string // reconstructed version suffix ("#id", "~2", or "")
}

// ShowTag represents a tag key-value pair.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	Name        string
	Value       string
	Version     string // opaque version id (Key Vault), or "" (App Configuration / latest)
	State       string // enabled/disabled (Key Vault, best-effort), may be ""
	CreatedDate *time.Time
	Tags        []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Reader provider.Reader
}

// Execute runs the show use case. Version resolution (opaque ids and ~shift for
// Key Vault; latest-only for App Configuration) is provided by the adapter
// behind provider.Reader.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	ref, err := u.Reader.Resolve(ctx, input.Name, input.Suffix)
	if err != nil {
		return nil, err
	}

	entry, err := u.Reader.Get(ctx, input.Name, ref)
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
