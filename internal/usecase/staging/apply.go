package staging

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// ApplyInput holds input for the apply use case.
type ApplyInput struct {
	Name            string // Optional: apply only this item
	IgnoreConflicts bool   // Skip conflict detection
}

// ApplyResultStatus represents the status of an apply operation.
type ApplyResultStatus int

// ApplyResultStatus constants representing the outcome of applying a staged entry.
const (
	ApplyResultCreated ApplyResultStatus = iota
	ApplyResultUpdated
	ApplyResultDeleted
	ApplyResultFailed
)

// ApplyEntryResult represents the result of applying a single entry.
type ApplyEntryResult struct {
	Name string
	// Namespace is the App Configuration namespace the entry was applied under
	// (empty for the null/default namespace and every other provider).
	Namespace string
	Status    ApplyResultStatus
	Error     error
	// UnstageError is set when the cloud apply succeeded but clearing the entry
	// from the staging store afterwards failed. The entry is still staged, so a
	// later apply would re-run it; callers must surface this rather than ignore it.
	UnstageError error
}

// ApplyTagResult represents the result of applying tag changes.
type ApplyTagResult struct {
	Name      string
	AddTags   map[string]string   // Tags that were added/updated
	RemoveTag maputil.Set[string] // Tag keys that were removed
	Error     error
	// UnstageError is set when the cloud tag apply succeeded but clearing the
	// staged tag afterwards failed (see ApplyEntryResult.UnstageError).
	UnstageError error
}

// ApplyOutput holds the result of the apply use case.
type ApplyOutput struct {
	ServiceName string
	ItemName    string
	// Entry results
	EntryResults   []ApplyEntryResult
	EntrySucceeded int
	EntryFailed    int
	// Tag results
	TagResults   []ApplyTagResult
	TagSucceeded int
	TagFailed    int
	// Conflicts
	Conflicts []string
}

// ApplyUseCase executes apply operations.
type ApplyUseCase struct {
	Strategy staging.ApplyStrategy
	Store    store.ReadWriteOperator
	// StrategyFor, when set, resolves the ApplyStrategy for a given namespace so
	// each staged entry is applied through a provider store scoped to its own
	// namespace (Azure App Configuration, whose settings share one staging store
	// across namespaces). When nil, Strategy applies every entry — the case for
	// namespace-agnostic providers (AWS, Google Cloud, Key Vault).
	StrategyFor func(namespace string) (staging.ApplyStrategy, error)
}

// strategyForNamespace returns the apply strategy scoped to the given namespace,
// falling back to the single Strategy when no resolver is configured.
func (u *ApplyUseCase) strategyForNamespace(namespace string) (staging.ApplyStrategy, error) {
	if u.StrategyFor == nil {
		return u.Strategy, nil
	}

	return u.StrategyFor(namespace)
}

// Execute runs the apply use case.
func (u *ApplyUseCase) Execute(ctx context.Context, input ApplyInput) (*ApplyOutput, error) {
	service := u.Strategy.Service()
	serviceName := u.Strategy.ServiceName()
	itemName := u.Strategy.ItemName()

	output := &ApplyOutput{
		ServiceName: serviceName,
		ItemName:    itemName,
	}

	// Get staged entries and tags
	stagedEntries, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}

	stagedTags, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}

	entries := stagedEntries[service]
	tags := stagedTags[service]

	// Filter by name if specified. Entries are keyed by the (name, namespace)
	// composite, so match on the decoded bare name — for App Configuration this
	// applies the named setting across every namespace it is staged under.
	if input.Name != "" {
		filteredEntries := make(map[string]staging.Entry)
		filteredTags := make(map[string]staging.TagEntry)

		for key, entry := range entries {
			if name, _ := staging.SplitEntryKey(key); name == input.Name {
				filteredEntries[key] = entry
			}
		}

		for key, tagEntry := range tags {
			if name, _ := staging.SplitEntryKey(key); name == input.Name {
				filteredTags[key] = tagEntry
			}
		}

		if len(filteredEntries) == 0 && len(filteredTags) == 0 {
			return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
		}

		entries = filteredEntries
		tags = filteredTags
	}

	if len(entries) == 0 && len(tags) == 0 {
		return output, nil
	}

	// Check for conflicts (only for entries, as tags don't have value conflicts)
	if !input.IgnoreConflicts && len(entries) > 0 {
		conflicts := staging.CheckConflicts(ctx, u.Strategy, entries)
		if len(conflicts) > 0 {
			for name := range conflicts {
				output.Conflicts = append(output.Conflicts, name)
			}

			return output, fmt.Errorf("apply rejected: %d conflict(s) detected", len(conflicts))
		}
	}

	// Apply entries
	if len(entries) > 0 {
		u.applyEntries(ctx, service, entries, output)
	}

	// Apply tags
	if len(tags) > 0 {
		u.applyTags(ctx, service, tags, output)
	}

	// Calculate total failures
	totalFailed := output.EntryFailed + output.TagFailed
	if totalFailed > 0 {
		return output, fmt.Errorf("applied %d entries, %d tags; failed %d entries, %d tags",
			output.EntrySucceeded, output.TagSucceeded, output.EntryFailed, output.TagFailed)
	}

	return output, nil
}

func (u *ApplyUseCase) applyEntries(ctx context.Context, service staging.Service, entries map[string]staging.Entry, output *ApplyOutput) {
	// Execute apply operations in parallel. Entries are keyed by the composite
	// (name, namespace); each is applied through the strategy scoped to its own
	// namespace (App Configuration) or the single strategy (other providers).
	results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, key string, entry staging.Entry) (staging.Operation, error) {
		name, _ := staging.SplitEntryKey(key)

		strat, err := u.strategyForNamespace(entry.Namespace)
		if err != nil {
			return entry.Operation, err
		}

		return entry.Operation, strat.Apply(ctx, name, entry)
	})

	// Collect results
	for key, entry := range entries {
		name, _ := staging.SplitEntryKey(key)
		result := results[key]
		resultEntry := ApplyEntryResult{
			Name:      name,
			Namespace: entry.Namespace,
		}

		if result.Err != nil {
			resultEntry.Status = ApplyResultFailed
			resultEntry.Error = result.Err
			output.EntryFailed++
		} else {
			switch result.Value {
			case staging.OperationCreate:
				resultEntry.Status = ApplyResultCreated
			case staging.OperationUpdate:
				resultEntry.Status = ApplyResultUpdated
			case staging.OperationDelete:
				resultEntry.Status = ApplyResultDeleted
			}
			// Unstage successful operations. A failure here leaves the entry
			// staged after a successful cloud apply, so record it rather than
			// discarding it — a silent leftover would be re-applied next time.
			if err := u.Store.UnstageEntry(ctx, service, name, entry.Namespace); err != nil {
				resultEntry.UnstageError = err
			}

			output.EntrySucceeded++
		}

		output.EntryResults = append(output.EntryResults, resultEntry)
	}
}

func (u *ApplyUseCase) applyTags(ctx context.Context, service staging.Service, tags map[string]staging.TagEntry, output *ApplyOutput) {
	// Execute tag apply operations in parallel
	results := parallel.ExecuteMap(ctx, tags, func(ctx context.Context, name string, tagEntry staging.TagEntry) (struct{}, error) {
		err := u.Strategy.ApplyTags(ctx, name, tagEntry)

		return struct{}{}, err
	})

	// Collect results
	for name, tagEntry := range tags {
		result := results[name]
		resultTag := ApplyTagResult{
			Name:      name,
			AddTags:   tagEntry.Add,
			RemoveTag: tagEntry.Remove,
		}

		if result.Err != nil {
			resultTag.Error = result.Err
			output.TagFailed++
		} else {
			// Unstage successful operations (see applyEntries: record rather
			// than discard a post-apply unstage failure).
			if err := u.Store.UnstageTag(ctx, service, name); err != nil {
				resultTag.UnstageError = err
			}

			output.TagSucceeded++
		}

		output.TagResults = append(output.TagResults, resultTag)
	}
}
