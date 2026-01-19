// Package resource provides unified use cases for versioned resources (parameters and secrets).
package resource

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// ShowClient is the interface for the show use case.
// It requires tag fetching capability.
type ShowClient interface {
	provider.Tagger
}

// ShowInput holds input for the show use case.
// The Resource should be pre-resolved from a version specification.
type ShowInput struct {
	// Resource is the resolved resource to display.
	// Use paramversion.Resolve or secretversion.Resolve to obtain this.
	Resource *model.Resource
}

// ShowTag represents a tag key-value pair in the output.
type ShowTag struct {
	Key   string
	Value string
}

// ShowOutput holds the result of the show use case.
type ShowOutput struct {
	// Kind identifies whether this is a parameter or secret.
	Kind model.ResourceKind
	// Name is the resource name/identifier.
	Name string
	// ARN is the resource ARN (primarily for secrets, empty for parameters).
	ARN string
	// Value is the resource value/content.
	Value string
	// Version is the version identifier.
	Version string
	// Type is the resource type (e.g., String, SecureString for parameters).
	// This is empty for secrets which don't have a type concept.
	Type string
	// Description is the resource description.
	Description string
	// ModifiedAt is when the resource was last modified.
	ModifiedAt *time.Time
	// Tags are key-value tags associated with the resource.
	Tags []ShowTag
	// Metadata contains provider-specific metadata for additional display.
	// CLI/GUI layers can type-assert this to extract provider-specific fields.
	Metadata any
}

// ShowUseCase executes show operations for versioned resources.
type ShowUseCase struct {
	Client ShowClient
}

// Execute runs the show use case.
// It takes a pre-resolved resource and fetches its tags.
func (u *ShowUseCase) Execute(ctx context.Context, input ShowInput) (*ShowOutput, error) {
	res := input.Resource

	output := &ShowOutput{
		Kind:        res.Kind,
		Name:        res.Name,
		ARN:         res.ARN,
		Value:       res.Value,
		Version:     res.Version,
		Type:        res.Type,
		Description: res.Description,
		ModifiedAt:  res.ModifiedAt,
		Metadata:    res.Metadata,
	}

	// Fetch tags
	tags, err := u.Client.GetTags(ctx, res.Name)
	if err == nil && tags != nil {
		output.Tags = make([]ShowTag, 0, len(tags))
		for k, v := range tags {
			output.Tags = append(output.Tags, ShowTag{
				Key:   k,
				Value: v,
			})
		}
	}

	return output, nil
}
