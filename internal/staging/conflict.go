package staging

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/parallel"
)

// ApplyStrategyResolver resolves the ApplyStrategy for a given namespace. It
// mirrors the per-namespace resolution the apply path uses so a namespaced
// provider (Azure App Configuration) probes each entry against the remote state
// of its OWN namespace rather than the default one.
type ApplyStrategyResolver func(namespace string) (ApplyStrategy, error)

// lastModifiedResults maps each probed EntryKey to its fetched last-modified
// time (or the fetch error).
type lastModifiedResults = map[EntryKey]*parallel.Result[time.Time]

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
	return CheckEntryAndTagConflicts(ctx, resolve, entries, nil)
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
	return CheckEntryAndTagConflicts(ctx, resolve, nil, tags)
}

// CheckEntryAndTagConflicts checks both staged value changes and staged tag
// changes for conflicts and returns the merged set of conflicting EntryKeys.
//
// It fetches each remote's last-modified time at most once, even when the same
// key carries both a value change and a tag change: the two probes previously
// double-fetched the same remote, wasting I/O and widening the window in which
// the two timestamps could disagree on second-granular providers. The value and
// tag comparisons now share that single fetch.
func CheckEntryAndTagConflicts(
	ctx context.Context,
	resolve ApplyStrategyResolver,
	entries map[EntryKey]Entry,
	tags map[EntryKey]TagEntry,
) map[EntryKey]struct{} {
	conflicts := make(map[EntryKey]struct{})

	toCheckCreate, toCheckModified := classifyEntries(entries)
	toCheckTags := tagsWithBase(tags)

	if len(toCheckCreate) == 0 && len(toCheckModified) == 0 && len(toCheckTags) == 0 {
		return conflicts
	}

	// Merge the key sets so each remote is fetched exactly once, then share the
	// results across the create, modified and tag comparisons below.
	keys := make(map[EntryKey]struct{}, len(toCheckCreate)+len(toCheckModified)+len(toCheckTags))
	addKeys(keys, toCheckCreate)
	addKeys(keys, toCheckModified)
	addKeys(keys, toCheckTags)

	results := fetchLastModified(ctx, resolve, keys)

	// Create: conflict if the resource now exists (someone else created it).
	for key := range toCheckCreate {
		result := results[key]
		if result.Err == nil && !result.Value.IsZero() {
			conflicts[key] = struct{}{}
		}
	}

	// Update/Delete and tag changes: conflict if the remote was modified after
	// the staged base time.
	markModifiedAfterBase(toCheckModified, func(e Entry) time.Time { return *e.BaseModifiedAt }, results, conflicts)
	markModifiedAfterBase(toCheckTags, func(t TagEntry) time.Time { return *t.BaseModifiedAt }, results, conflicts)

	return conflicts
}

// classifyEntries splits entries into those checked for a Create conflict (the
// resource now exists) and those checked for a modified-after-base conflict.
// Entries without a check type (Update/Delete lacking BaseModifiedAt) are dropped.
func classifyEntries(entries map[EntryKey]Entry) (create, modified map[EntryKey]Entry) {
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

// tagsWithBase returns the tag changes that carry a BaseModifiedAt and can
// therefore be conflict-checked; the rest are never conflicts.
func tagsWithBase(tags map[EntryKey]TagEntry) map[EntryKey]TagEntry {
	toCheck := make(map[EntryKey]TagEntry)

	for key, tag := range tags {
		if tag.BaseModifiedAt != nil {
			toCheck[key] = tag
		}
	}

	return toCheck
}

// addKeys copies the keys of src into dst.
func addKeys[V any](dst map[EntryKey]struct{}, src map[EntryKey]V) {
	for key := range src {
		dst[key] = struct{}{}
	}
}

// fetchLastModified fetches each key's remote last-modified time in parallel,
// resolving the strategy for the key's own namespace so a namespaced provider
// probes each entry against the right remote.
func fetchLastModified(ctx context.Context, resolve ApplyStrategyResolver, keys map[EntryKey]struct{}) lastModifiedResults {
	return parallel.ExecuteMap(ctx, keys, func(ctx context.Context, key EntryKey, _ struct{}) (time.Time, error) {
		strategy, err := resolve(key.Namespace)
		if err != nil {
			return time.Time{}, err
		}

		return strategy.FetchLastModified(ctx, key.Name)
	})
}

// markModifiedAfterBase adds to conflicts every key whose remote was modified
// strictly after its staged base time. A fetch error or a zero time (the remote
// no longer exists) is skipped — the apply will fail on its own. base extracts
// each item's staged base time.
//
// Strict After: on second-granular providers (e.g. Azure Key Vault) an
// out-of-band write in the same wall-clock second compares as equal and escapes
// detection. See docs/staging-state-transitions.md.
func markModifiedAfterBase[V any](
	items map[EntryKey]V,
	base func(V) time.Time,
	results lastModifiedResults,
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
