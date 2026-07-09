package staging

import (
	"context"
	"maps"
	"time"

	"github.com/mpyw/suve/internal/parallel"
)

// CheckConflicts checks if remote resources were modified after staging.
// Returns the set of EntryKeys that have conflicts.
//
// For Create operations: conflicts if resource now exists (someone else created it).
// For Update/Delete operations with BaseModifiedAt: conflicts if the remote was modified after base.
func CheckConflicts(ctx context.Context, strategy ApplyStrategy, entries map[EntryKey]Entry) map[EntryKey]struct{} {
	conflicts := make(map[EntryKey]struct{})

	// Separate entries by check type:
	// - Create: check if resource now exists (someone else created it)
	// - Update/Delete with BaseModifiedAt: check if modified after base
	toCheckCreate := make(map[EntryKey]Entry)
	toCheckModified := make(map[EntryKey]Entry)

	for key, entry := range entries {
		switch {
		case entry.Operation == OperationCreate:
			toCheckCreate[key] = entry
		case (entry.Operation == OperationUpdate || entry.Operation == OperationDelete) && entry.BaseModifiedAt != nil:
			toCheckModified[key] = entry
		}
	}

	if len(toCheckCreate) == 0 && len(toCheckModified) == 0 {
		return conflicts
	}

	// Combine all entries for parallel fetch
	allToCheck := make(map[EntryKey]Entry)

	maps.Copy(allToCheck, toCheckCreate)
	maps.Copy(allToCheck, toCheckModified)

	// Fetch last modified times in parallel
	results := parallel.ExecuteMap(ctx, allToCheck, func(ctx context.Context, key EntryKey, _ Entry) (time.Time, error) {
		return strategy.FetchLastModified(ctx, key.Name)
	})

	// Check for conflicts - Create operations
	for key := range toCheckCreate {
		result := results[key]
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on apply anyway)
			continue
		}

		// For Create: if resource now exists (non-zero time), someone else created it
		if !result.Value.IsZero() {
			conflicts[key] = struct{}{}
		}
	}

	// Check for conflicts - Update/Delete operations
	for key, entry := range toCheckModified {
		result := results[key]
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
			conflicts[key] = struct{}{}
		}
	}

	return conflicts
}
