// Package tagging is the provider-neutral tag/untag use case shared by the
// param and secret services.
package tagging

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
)

// Input holds input for the tag use case.
type Input struct {
	Name   string
	Add    map[string]string // Tags to add or update
	Remove []string          // Tag keys to remove
}

// UseCase executes tag operations. Additions are applied before removals.
type UseCase struct {
	Tagger provider.Tagger
}

// Execute runs the tag use case.
func (u *UseCase) Execute(ctx context.Context, input Input) error {
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
