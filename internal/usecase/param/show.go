// Package param provides use cases for SSM Parameter Store operations.
package param

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/awsparamversion"
)

// ShowInput holds input for the show use case.
type ShowInput struct {
	Spec *awsparamversion.Spec
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
	Version      int64
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
		Value:        entry.Value,
		Version:      parseVersion(entry.Version.ID),
		Type:         entry.Type,
		Description:  entry.Description,
		LastModified: entry.Modified,
		Tags: lo.Map(entry.Tags, func(tag domain.Tag, _ int) ShowTag {
			return ShowTag{Key: tag.Key, Value: tag.Value}
		}),
	}

	return output, nil
}
