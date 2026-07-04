package secret

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
)

// TagInput holds input for the tag use case.
type TagInput struct {
	Name   string
	Add    map[string]string // Tags to add or update
	Remove []string          // Tag keys to remove
}

// TagUseCase executes tag operations.
type TagUseCase struct {
	Tagger provider.Tagger
}

// Execute runs the tag use case.
func (u *TagUseCase) Execute(ctx context.Context, input TagInput) error {
	if len(input.Add) > 0 {
		if err := u.Tagger.Tag(ctx, input.Name, input.Add); err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	if len(input.Remove) > 0 {
		if err := u.Tagger.Untag(ctx, input.Name, input.Remove); err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}
