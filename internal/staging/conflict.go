package staging

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/parallel"
)

// conflictLastModifiedResults maps each probed EntryKey to its fetched last-modified
// time (or the fetch error).
type conflictLastModifiedResults = map[EntryKey]*parallel.Result[time.Time]

// CheckEntryAndTagConflicts checks staged value changes and staged tag changes
// for conflicts and returns the merged set of conflicting EntryKeys.
//
//   - Create: conflicts if the resource now exists (someone else created it).
//   - Update/Delete with BaseModifiedAt: conflicts if the remote was modified
//     after the base time.
//   - Tag change with BaseModifiedAt: conflicts if the remote was modified after
//     the tags were fetched. A tag change without a base time cannot be checked,
//     and a remote that no longer exists is skipped (the tag apply fails on its
//     own).
//
// Each key is probed through the strategy resolved for its own namespace, so
// two same-named entries in different App Configuration namespaces never
// collapse onto one namespace's remote state; namespace-agnostic providers
// resolve the single strategy under the empty namespace. Each remote's
// last-modified time is fetched at most once, even when a key carries both a
// value change and a tag change.
func CheckEntryAndTagConflicts(
	ctx context.Context,
	resolve ApplyStrategyResolver,
	entries map[EntryKey]Entry,
	tags map[EntryKey]TagEntry,
) map[EntryKey]struct{} {
	conflicts := make(map[EntryKey]struct{})

	toCheckCreate, toCheckModified := classifyConflictEntries(entries)
	toCheckTags := conflictTagsWithBase(tags)

	if len(toCheckCreate) == 0 && len(toCheckModified) == 0 && len(toCheckTags) == 0 {
		return conflicts
	}

	// Merge the key sets so each remote is fetched exactly once, then share the
	// results across the create, modified and tag comparisons below.
	keys := make(map[EntryKey]struct{}, len(toCheckCreate)+len(toCheckModified)+len(toCheckTags))
	addConflictKeys(keys, toCheckCreate)
	addConflictKeys(keys, toCheckModified)
	addConflictKeys(keys, toCheckTags)

	results := fetchConflictLastModified(ctx, resolve, keys)

	// Create: conflict if the resource now exists (someone else created it).
	for key := range toCheckCreate {
		result := results[key]
		if result.Err == nil && !result.Value.IsZero() {
			conflicts[key] = struct{}{}
		}
	}

	// Update/Delete and tag changes: conflict if the remote was modified after
	// the staged base time.
	markConflictsModifiedAfterBase(toCheckModified, func(e Entry) time.Time { return *e.BaseModifiedAt }, results, conflicts)
	markConflictsModifiedAfterBase(toCheckTags, func(t TagEntry) time.Time { return *t.BaseModifiedAt }, results, conflicts)

	return conflicts
}

// classifyConflictEntries splits entries into those checked for a Create conflict (the
// resource now exists) and those checked for a modified-after-base conflict.
// Entries without a check type (Update/Delete lacking BaseModifiedAt) are dropped.
func classifyConflictEntries(entries map[EntryKey]Entry) (create, modified map[EntryKey]Entry) {
	create = make(map[EntryKey]Entry)
	modified = make(map[EntryKey]Entry)

	for key, entry := range entries {
		switch {
		case entry.Operation == OperationCreate:
			create[key] = entry
		case (entry.Operation == OperationUpdate || entry.Operation == OperationDelete) && entry.BaseModifiedAt != nil:
			modified[key] = entry
		}
	}

	return create, modified
}

// conflictTagsWithBase returns the tag changes that carry a BaseModifiedAt and can
// therefore be conflict-checked; the rest are never conflicts.
func conflictTagsWithBase(tags map[EntryKey]TagEntry) map[EntryKey]TagEntry {
	toCheck := make(map[EntryKey]TagEntry)

	for key, tag := range tags {
		if tag.BaseModifiedAt != nil {
			toCheck[key] = tag
		}
	}

	return toCheck
}

// addConflictKeys copies the keys of src into dst.
func addConflictKeys[V any](dst map[EntryKey]struct{}, src map[EntryKey]V) {
	for key := range src {
		dst[key] = struct{}{}
	}
}

// fetchConflictLastModified fetches each key's remote last-modified time in parallel,
// resolving the strategy for the key's own namespace so a namespaced provider
// probes each entry against the right remote.
func fetchConflictLastModified(ctx context.Context, resolve ApplyStrategyResolver, keys map[EntryKey]struct{}) conflictLastModifiedResults {
	return parallel.ExecuteMap(ctx, keys, func(ctx context.Context, key EntryKey, _ struct{}) (time.Time, error) {
		strategy, err := resolve(key.Namespace)
		if err != nil {
			return time.Time{}, err
		}

		return strategy.FetchLastModified(ctx, key.Name)
	})
}

// markConflictsModifiedAfterBase adds to conflicts every key whose remote was modified
// strictly after its staged base time. A fetch error or a zero time (the remote
// no longer exists) is skipped — the apply will fail on its own. base extracts
// each item's staged base time.
//
// Strict After: on second-granular providers (e.g. Azure Key Vault) an
// out-of-band write in the same wall-clock second compares as equal and escapes
// detection. See docs/staging-state-transitions.md.
func markConflictsModifiedAfterBase[V any](
	items map[EntryKey]V,
	base func(V) time.Time,
	results conflictLastModifiedResults,
	conflicts map[EntryKey]struct{},
) {
	for key, item := range items {
		result := results[key]
		if result.Err != nil || result.Value.IsZero() {
			continue
		}

		if result.Value.After(base(item)) {
			conflicts[key] = struct{}{}
		}
	}
}
