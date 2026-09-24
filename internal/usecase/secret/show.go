// Package secret provides the provider-neutral use cases of the secret service
// axis (AWS Secrets Manager, Google Cloud Secret Manager, Azure Key Vault).
//
// The use cases speak only the neutral provider seam: an entry is addressed by
// its name plus an opaque version suffix (e.g. "#abc", ":LABEL", "~2", or ""
// for the current version) that the caller rebuilt with its provider's grammar
// and that the adapter re-parses in Reader.Resolve. Versions come back as
// opaque ids; provider-specific rendering (ARN display, id truncation) belongs
// to the presenters.
package secret

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
	// Suffix is the version-spec suffix after the name ("#id", ":LABEL", "~2",
	// or "" for the current version), re-parsed by the adapter.
	Suffix string
}

// ShowTag represents a tag key-value pair.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
//
// Labels and State carry the two independent, provider-specific axes that must
// NOT be conflated (#419): Labels holds the version's movable labels (AWS
// Secrets Manager staging labels; empty for other providers), while State holds
// the per-version lifecycle state (enabled/disabled/destroyed) for Google Cloud
// + Azure Key Vault (empty for AWS). A version never has both.
type ShowOutput struct {
	Name        string
	Value       string
	Version     string // opaque version id
	Labels      []string
	State       string
	Description string
	CreatedDate *time.Time
	Tags        []ShowTag
	// Extra is the provider's display-only metadata (e.g. the AWS Secrets Manager
	// ARN), passed through verbatim from the entry.
	Extra []domain.Field
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Reader provider.Reader
}

// Execute runs the show use case. Version/label/shift resolution is provided by
// the adapter behind provider.Reader.
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
		Labels:      versionLabels(entry.Version.Labels),
		State:       entry.Version.State,
		Description: entry.Description,
		CreatedDate: entry.Version.Created,
		Tags: lo.Map(entry.Tags, func(tag domain.Tag, _ int) ShowTag {
			return ShowTag{Key: tag.Key, Value: tag.Value}
		}),
		Extra: entry.Extra,
	}, nil
}
