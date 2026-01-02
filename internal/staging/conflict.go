package staging

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/parallel"
)

// CheckConflicts checks if AWS resources were modified after staging.
// Returns a map of names that have conflicts.
//
// For Create operations: conflicts if resource now exists (someone else created it).
// For Update/Delete operations with BaseModifiedAt: conflicts if AWS was modified after base.
func CheckConflicts(ctx context.Context, strategy ApplyStrategy, entries map[string]Entry) map[string]struct{} {
	conflicts := make(map[string]struct{})

	// Separate entries by check type:
	// - Create: check if resource now exists (someone else created it)
	// - Update/Delete with BaseModifiedAt: check if modified after base
	toCheckCreate := make(map[string]Entry)
	toCheckModified := make(map[string]Entry)

	for name, entry := range entries {
		switch {
		case entry.Operation == OperationCreate:
			toCheckCreate[name] = entry
		case (entry.Operation == OperationUpdate || entry.Operation == OperationDelete) && entry.BaseModifiedAt != nil:
			toCheckModified[name] = entry
		}
	}

	if len(toCheckCreate) == 0 && len(toCheckModified) == 0 {
		return conflicts
	}

	// Combine all entries for parallel fetch
	allToCheck := make(map[string]Entry)
	for name, entry := range toCheckCreate {
		allToCheck[name] = entry
	}
	for name, entry := range toCheckModified {
		allToCheck[name] = entry
	}

	// Fetch last modified times in parallel
	results := parallel.ExecuteMap(ctx, allToCheck, func(ctx context.Context, name string, _ Entry) (time.Time, error) {
		return strategy.FetchLastModified(ctx, name)
	})

	// Check for conflicts - Create operations
	for name := range toCheckCreate {
		result := results[name]
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on apply anyway)
			continue
		}

		// For Create: if resource now exists (non-zero time), someone else created it
		if !result.Value.IsZero() {
			conflicts[name] = struct{}{}
		}
	}

	// Check for conflicts - Update/Delete operations
	for name, entry := range toCheckModified {
		result := results[name]
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on apply anyway)
			continue
		}

		awsModified := result.Value

		// Zero time means resource doesn't exist - no conflict for delete (already gone)
		if awsModified.IsZero() {
			continue
		}

		// If AWS was modified after the base value was fetched, it's a conflict
		if awsModified.After(*entry.BaseModifiedAt) {
			conflicts[name] = struct{}{}
		}
	}

	return conflicts
}
