package staging

import (
	"context"
	"maps"
	"time"

	"github.com/mpyw/suve/internal/parallel"
)

// ApplyStrategyResolver resolves the ApplyStrategy for a given namespace. It
// mirrors the per-namespace resolution the apply path uses so a namespaced
// provider (Azure App Configuration) probes each entry against the remote state
// of its OWN namespace rather than the default one.
type ApplyStrategyResolver func(namespace string) (ApplyStrategy, error)

// CheckConflicts checks if remote resources were modified after staging.
// Returns the set of EntryKeys that have conflicts.
//
// Each entry is probed through the strategy resolved for its own namespace, so
// the probe carries the full EntryKey (name + namespace) and never collapses
// two same-named entries across namespaces onto one namespace's remote state.
// For namespace-agnostic providers the resolver returns the single strategy and
// the empty namespace, so behavior is unchanged.
//
// For Create operations: conflicts if resource now exists (someone else created it).
// For Update/Delete operations with BaseModifiedAt: conflicts if the remote was modified after base.
func CheckConflicts(ctx context.Context, resolve ApplyStrategyResolver, entries map[EntryKey]Entry) map[EntryKey]struct{} {
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

	// Fetch last modified times in parallel. Resolve the strategy per entry so the
	// probe targets the entry's own namespace (App Configuration); other providers
	// resolve the same single strategy under the empty namespace.
	results := parallel.ExecuteMap(ctx, allToCheck, func(ctx context.Context, key EntryKey, _ Entry) (time.Time, error) {
		strategy, err := resolve(key.Namespace)
		if err != nil {
			return time.Time{}, err
		}

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
