package secret

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
)

// TagClient is the interface for the tag use case.
type TagClient interface {
	provider.SecretTagger
}

// TagInput holds input for the tag use case.
type TagInput struct {
	Name   string
	Add    map[string]string // Tags to add or update
	Remove []string          // Tag keys to remove
}

// TagUseCase executes tag operations.
type TagUseCase struct {
	Client TagClient
}

// Execute runs the tag use case.
func (u *TagUseCase) Execute(ctx context.Context, input TagInput) error {
	// Add tags
	if len(input.Add) > 0 {
		if err := u.Client.AddTags(ctx, input.Name, input.Add); err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	// Remove tags
	if len(input.Remove) > 0 {
		if err := u.Client.RemoveTags(ctx, input.Name, input.Remove); err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
