// The all-service apply shares apply.go's listing, conflict check and apply
// steps, so it joins the apply namespace.
//declscope:namespace apply

package staging

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/staging"
)

// GlobalApplyInput holds input for the all-service apply use case.
type GlobalApplyInput struct {
	IgnoreConflicts bool // Skip conflict detection
}

// GlobalApplyConflict is one conflicting staged item, qualified by its service.
// Conflicts are tracked per (service, key) rather than per key alone, so two
// services staging the same (name, namespace) — e.g. an SSM param and a Secrets
// Manager secret both named "foo" — are reported separately.
type GlobalApplyConflict struct {
	ServiceName string
	Key         staging.EntryKey
}

// GlobalApplyOutput holds the result of the all-service apply use case.
type GlobalApplyOutput struct {
	// Services holds one ApplyOutput per service that had staged changes, in the
	// use case's service order. It is empty when the apply was rejected.
	Services []*ApplyOutput
	// Conflicts lists every conflict in service order, then (name, namespace)
	// order. A non-empty list means nothing was applied.
	Conflicts []GlobalApplyConflict
	// Succeeded and Failed count entry and tag applies across every service.
	Succeeded int
	Failed    int
}

// GlobalApplyUseCase applies the staged changes of several services of one
// provider as a single operation: every service is conflict-checked before any
// of them is applied, then value changes are applied service by service,
// followed by tag changes service by service.
type GlobalApplyUseCase struct {
	// Services lists one per-service apply use case per service, in stable
	// apply order. Each carries its own store and strategy.
	Services []*ApplyUseCase
}

// globalApplyStaged is one service's staged changes, listed once and shared by
// the conflict check and the apply.
type globalApplyStaged struct {
	useCase *ApplyUseCase
	entries map[staging.EntryKey]staging.Entry
	tags    map[staging.EntryKey]staging.TagEntry
	output  *ApplyOutput
}

// Execute runs the all-service apply use case.
func (u *GlobalApplyUseCase) Execute(ctx context.Context, input GlobalApplyInput) (*GlobalApplyOutput, error) {
	var staged []globalApplyStaged

	for _, uc := range u.Services {
		entries, tags, err := uc.staged(ctx, "")
		if err != nil {
			return nil, err
		}

		if len(entries) == 0 && len(tags) == 0 {
			continue
		}

		staged = append(staged, globalApplyStaged{
			useCase: uc,
			entries: entries,
			tags:    tags,
			output: &ApplyOutput{
				ServiceName: uc.Strategy.ServiceName(),
				ItemName:    uc.Strategy.ItemName(),
			},
		})
	}

	output := &GlobalApplyOutput{}

	if !input.IgnoreConflicts {
		var checkErrs []error

		for _, s := range staged {
			keys, err := s.useCase.conflicts(ctx, s.entries, s.tags)
			if err != nil {
				checkErrs = append(checkErrs, fmt.Errorf("%s: %w", s.output.ServiceName, err))

				continue
			}

			for _, key := range keys {
				output.Conflicts = append(output.Conflicts, GlobalApplyConflict{ServiceName: s.output.ServiceName, Key: key})
			}
		}

		if len(checkErrs) > 0 {
			return nil, applyConflictCheckError(errors.Join(checkErrs...))
		}

		if len(output.Conflicts) > 0 {
			return output, fmt.Errorf("apply rejected: %d conflict(s) detected", len(output.Conflicts))
		}
	}

	// Apply every service's value changes before any service's tag changes.
	for _, s := range staged {
		if len(s.entries) > 0 {
			s.useCase.applyEntries(ctx, s.useCase.Strategy.Service(), s.entries, s.output)
		}
	}

	for _, s := range staged {
		if len(s.tags) > 0 {
			s.useCase.applyTags(ctx, s.useCase.Strategy.Service(), s.tags, s.output)
		}

		output.Services = append(output.Services, s.output)
		output.Succeeded += s.output.EntrySucceeded + s.output.TagSucceeded
		output.Failed += s.output.EntryFailed + s.output.TagFailed
	}

	if output.Failed > 0 {
		return output, fmt.Errorf("applied %d, failed %d", output.Succeeded, output.Failed)
	}

	return output, nil
}
