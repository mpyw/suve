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

		// If AWS was modified after the base value was fetched, it's a conflict.
		// Strict After: on second-granular providers (e.g. Azure Key Vault) an
		// out-of-band write in the same wall-clock second compares as equal and
		// escapes detection. See docs/staging-state-transitions.md.
		if awsModified.After(*entry.BaseModifiedAt) {
			conflicts[key] = struct{}{}
		}
	}

	return conflicts
}

// CheckTagConflicts checks if the remote resource behind each staged tag change
// was modified after the tags were fetched. Returns the set of EntryKeys that
// have conflicts.
//
// It mirrors the Update/Delete path of CheckConflicts using the tag's own
// TagEntry.BaseModifiedAt: if the remote's last-modified time is after that base
// time, someone changed the resource since the tags were staged and the apply is
// a conflict rather than a silent overwrite. Each tag change is probed through
// the strategy resolved for its own namespace (App Configuration); other
// providers resolve the single strategy under the empty namespace.
//
// A tag change with no BaseModifiedAt cannot be checked and is never a conflict.
// A remote that no longer exists (zero time) is skipped too — the tag apply will
// fail on its own.
func CheckTagConflicts(ctx context.Context, resolve ApplyStrategyResolver, tags map[EntryKey]TagEntry) map[EntryKey]struct{} {
	conflicts := make(map[EntryKey]struct{})

	toCheck := make(map[EntryKey]TagEntry)

	for key, tag := range tags {
		if tag.BaseModifiedAt != nil {
			toCheck[key] = tag
		}
	}

	if len(toCheck) == 0 {
		return conflicts
	}

	// Fetch last modified times in parallel, resolving the strategy per entry so
	// the probe targets the tagged item's own namespace.
	results := parallel.ExecuteMap(ctx, toCheck, func(ctx context.Context, key EntryKey, _ TagEntry) (time.Time, error) {
		strategy, err := resolve(key.Namespace)
		if err != nil {
			return time.Time{}, err
		}

		return strategy.FetchLastModified(ctx, key.Name)
	})

	for key, tag := range toCheck {
		result := results[key]
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on apply anyway)
			continue
		}

		awsModified := result.Value

		// Zero time means the resource no longer exists - the tag apply will fail
		// on its own, so don't report it as a conflict here.
		if awsModified.IsZero() {
			continue
		}

		// If the remote was modified after the tags were fetched, it's a conflict.
		if awsModified.After(*tag.BaseModifiedAt) {
			conflicts[key] = struct{}{}
		}
	}

	return conflicts
}
