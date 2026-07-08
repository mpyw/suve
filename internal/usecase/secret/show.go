// Package secret provides use cases for Secrets Manager operations.
package secret

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *secretversion.Spec
}

// ShowTag represents a tag key-value pair.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
//
// VersionStage and State carry the two independent, provider-specific axes that
// must NOT be conflated (#419): VersionStage holds AWS Secrets Manager staging
// labels (empty for other providers), while State holds the per-version
// lifecycle state (enabled/disabled/destroyed) for Google Cloud + Azure Key
// Vault (empty for AWS). A version never has both.
type ShowOutput struct {
	Name         string
	ARN          string
	Value        string
	VersionID    string
	VersionStage []string
	State        string
	Description  string
	CreatedDate  *time.Time
	Tags         []ShowTag
}

// ShowUseCase executes show operations.
type ShowUseCase struct {
	Reader provider.Reader
}

// Execute runs the show use case. Version/label/shift resolution and the ARN
// (surfaced via the entry's Extra metadata) are provided by the adapter behind
// provider.Reader.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	ref, err := u.Reader.Resolve(ctx, input.Spec.Name, specSuffix(input.Spec))
	if err != nil {
		return nil, err
	}

	entry, err := u.Reader.Get(ctx, input.Spec.Name, ref)
	if err != nil {
		return nil, err
	}

	output := &ShowOutput{
		Name:         entry.Name,
		ARN:          extraValue(entry, "ARN"),
		Value:        entry.Value,
		VersionID:    entry.Version.ID,
		VersionStage: stages(entry.Version.StagingLabels),
		State:        entry.Version.State,
		Description:  entry.Description,
		CreatedDate:  entry.Version.Created,
		Tags: lo.Map(entry.Tags, func(tag domain.Tag, _ int) ShowTag {
			return ShowTag{Key: tag.Key, Value: tag.Value}
		}),
	}

	return output, nil
}
