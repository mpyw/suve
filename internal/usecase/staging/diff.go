package staging

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// DiffInput holds input for the diff use case.
type DiffInput struct {
	Name string // Optional: diff only this item
}

// DiffEntryType represents the type of diff entry.
type DiffEntryType int

// DiffEntryType constants representing the type of diff result.
const (
	DiffEntryNormal DiffEntryType = iota
	DiffEntryCreate
	DiffEntryAutoUnstaged
	DiffEntryWarning
)

// DiffEntry represents a single diff result for entries.
type DiffEntry struct {
	Name string
	// Namespace is the App Configuration namespace of the entry (empty for the
	// null/default namespace and every other provider).
	Namespace        string
	Type             DiffEntryType
	Operation        staging.Operation
	RemoteValue      string
	RemoteIdentifier string
	StagedValue      string
	Description      *string
	Warning          string // For warnings like "already deleted in Key Vault"
	// Secret reports whether the entry's values are secret material (a secret
	// service, or a SecureString param), so a consumer masks both the remote and
	// staged values in the review. Keyed off the value type, not the service, so
	// a SecureString param is masked too (#677).
	Secret bool
}

// DiffTagEntry represents a single diff result for tag changes.
type DiffTagEntry struct {
	Name string
	// Namespace is the App Configuration namespace of the tagged item (empty for
	// the null/default namespace and every other provider).
	Namespace string
	Add       map[string]string // Tags to add or update
	Remove    map[string]string // Tags to remove (key=current remote value)
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	ItemName   string
	Entries    []DiffEntry
	TagEntries []DiffTagEntry
}

// DiffUseCase executes diff operations.
type DiffUseCase struct {
	Strategy staging.DiffStrategy
	Store    store.ReadWriteOperator
	// StrategyFor, when set, resolves the DiffStrategy for a given namespace so
	// each staged entry is diffed against a provider store scoped to its own
	// namespace (Azure App Configuration). When nil, Strategy handles every
	// entry (namespace-agnostic providers).
	StrategyFor func(namespace string) (staging.DiffStrategy, error)
	// RemoteLabel names the remote in auto-unstage warnings (e.g. "already
	// deleted in AWS"). Empty uses the strategy's ServiceName.
	RemoteLabel string
}

// remoteLabel returns RemoteLabel, or the strategy's ServiceName when unset.
func (u *DiffUseCase) remoteLabel() string {
	if u.RemoteLabel != "" {
		return u.RemoteLabel
	}

	return u.Strategy.ServiceName()
}

// strategyForNamespace returns the diff strategy scoped to the given namespace,
// falling back to the single Strategy when no resolver is configured.
func (u *DiffUseCase) strategyForNamespace(namespace string) (staging.DiffStrategy, error) {
	if u.StrategyFor == nil {
		return u.Strategy, nil
	}

	return u.StrategyFor(namespace)
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	service := u.Strategy.Service()
	itemName := u.Strategy.ItemName()

	output := &DiffOutput{
		ItemName: itemName,
	}

	// Get all staged entries for the service
	allEntries, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}

	entries := allEntries[service]

	// Get all staged tag entries for the service
	allTagEntries, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}

	tagEntries := allTagEntries[service]

	// Filter by name if specified. Items are keyed by EntryKey (name, namespace),
	// so match on the key's name — for App Configuration this diffs the named
	// setting across every namespace it is staged under.
	if input.Name != "" {
		filteredEntries := make(map[staging.EntryKey]staging.Entry)
		filteredTags := make(map[staging.EntryKey]staging.TagEntry)

		for key, entry := range entries {
			if key.Name == input.Name {
				filteredEntries[key] = entry
			}
		}

		for key, tagEntry := range tagEntries {
			if key.Name == input.Name {
				filteredTags[key] = tagEntry
			}
		}

		// If neither exists, return warning
		if len(filteredEntries) == 0 && len(filteredTags) == 0 {
			output.Entries = append(output.Entries, DiffEntry{
				Name:    input.Name,
				Type:    DiffEntryWarning,
				Warning: "not staged",
			})

			return output, nil
		}

		entries = filteredEntries
		tagEntries = filteredTags
	}

	// Process entries
	// gone collects the keys auto-unstaged because the remote no longer exists;
	// their staged tag changes were discarded with them, so no tag row is shown.
	gone := make(map[staging.EntryKey]struct{})

	if len(entries) > 0 {
		// Fetch all current values in parallel, each through the strategy scoped
		// to its entry's namespace (App Configuration) or the single strategy.
		results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, key staging.EntryKey, _ staging.Entry) (*staging.FetchResult, error) {
			strategy, err := u.strategyForNamespace(key.Namespace)
			if err != nil {
				return nil, err
			}

			return strategy.FetchCurrent(ctx, key.Name)
		})

		// Process results
		for key, entry := range entries {
			result := results[key]

			diffEntry, err := u.processDiffResult(ctx, key, entry, result, gone)
			if err != nil {
				return nil, err
			}

			output.Entries = append(output.Entries, diffEntry)
		}
	}

	// Process tag entries - fetch current values for removed tags
	for key, tagEntry := range tagEntries {
		if _, ok := gone[key]; ok {
			continue
		}

		// A key with only tag changes had no entry probe above, so check here
		// that its remote still exists (see discardVanishedTags).
		if _, hasEntry := entries[key]; !hasEntry {
			diffEntry, discarded, err := u.discardVanishedTags(ctx, key)
			if err != nil {
				return nil, err
			}

			if discarded {
				output.Entries = append(output.Entries, diffEntry)

				continue
			}
		}

		diffTagEntry := DiffTagEntry{
			Name:      key.Name,
			Namespace: key.Namespace,
			Add:       tagEntry.Add,
		}

		// Fetch current tag values for removed tags
		if tagEntry.Remove.Len() > 0 {
			strategy, err := u.strategyForNamespace(key.Namespace)
			if err != nil {
				return nil, err
			}

			currentTags, err := strategy.FetchCurrentTags(ctx, key.Name)
			if err != nil {
				// If fetch fails, use keys only (fallback to old behavior)
				diffTagEntry.Remove = make(map[string]string, tagEntry.Remove.Len())
				for key := range tagEntry.Remove {
					diffTagEntry.Remove[key] = ""
				}
			} else {
				diffTagEntry.Remove = make(map[string]string, tagEntry.Remove.Len())
				for key := range tagEntry.Remove {
					if currentTags != nil {
						diffTagEntry.Remove[key] = currentTags[key]
					} else {
						diffTagEntry.Remove[key] = ""
					}
				}
			}
		}

		output.TagEntries = append(output.TagEntries, diffTagEntry)
	}

	return output, nil
}

//nolint:lll // function parameters are descriptive for clarity
func (u *DiffUseCase) processDiffResult(ctx context.Context, key staging.EntryKey, entry staging.Entry, result *parallel.Result[*staging.FetchResult], gone map[staging.EntryKey]struct{}) (DiffEntry, error) {
	service := u.Strategy.Service()

	if result.Err != nil {
		return u.handleFetchError(ctx, key, entry, result.Err, gone)
	}

	fetchResult := result.Value
	remoteValue := fetchResult.Value
	stagedValue := lo.FromPtr(entry.Value)

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Check if identical and auto-unstage. This never applies to a delete:
	// deleting a resource is not a no-op just because its current value happens
	// to be the empty string (legal in Azure App Configuration / Key Vault /
	// Google Cloud), so an empty-valued delete must not be silently cancelled.
	if entry.Operation != staging.OperationDelete && remoteValue == stagedValue {
		// A failed unstage would leave the entry staged while we report it as
		// auto-unstaged, so surface the error rather than discarding it.
		if err := u.Store.UnstageEntry(ctx, service, key); err != nil {
			return DiffEntry{}, fmt.Errorf("failed to unstage %s: %w", key.Name, err)
		}

		return DiffEntry{
			Name:      key.Name,
			Namespace: key.Namespace,
			Type:      DiffEntryAutoUnstaged,
			Warning:   "identical to " + u.remoteLabel() + " current",
		}, nil
	}

	return DiffEntry{
		Name:             key.Name,
		Namespace:        key.Namespace,
		Type:             DiffEntryNormal,
		Operation:        entry.Operation,
		RemoteValue:      remoteValue,
		RemoteIdentifier: fetchResult.Identifier,
		StagedValue:      stagedValue,
		Description:      entry.Description,
		Secret:           fetchResult.Secret,
	}, nil
}

// handleFetchError turns a failed remote fetch into a diff row. A not-found
// remote auto-unstages a staged Delete or Update together with its staged tag
// changes (a tag apply on a missing resource would fail on every later apply)
// and records the key in gone.
//
//nolint:lll // function parameters are descriptive for clarity
func (u *DiffUseCase) handleFetchError(ctx context.Context, key staging.EntryKey, entry staging.Entry, err error, gone map[staging.EntryKey]struct{}) (DiffEntry, error) {
	service := u.Strategy.Service()

	// Only a genuine "not found" justifies auto-unstaging a staged delete or
	// update. Any other fetch error (expired credentials, throttling, a network
	// blip) must NOT discard staged work on a read-only `stage diff`: surface it
	// as a warning and leave the staged entry untouched.
	notFound := errors.Is(err, provider.ErrNotFound)

	switch entry.Operation {
	case staging.OperationDelete:
		if notFound {
			note, uerr := u.unstageGone(ctx, service, key, gone)
			if uerr != nil {
				return DiffEntry{}, uerr
			}

			return DiffEntry{
				Name:      key.Name,
				Namespace: key.Namespace,
				Type:      DiffEntryAutoUnstaged,
				Warning:   "already deleted in " + u.remoteLabel() + note,
			}, nil
		}

	case staging.OperationCreate:
		return DiffEntry{
			Name:        key.Name,
			Namespace:   key.Namespace,
			Type:        DiffEntryCreate,
			Operation:   entry.Operation,
			StagedValue: lo.FromPtr(entry.Value),
			Description: entry.Description,
			// A create has no remote to fetch, so derive Secret from the staged
			// value type: a SecureString param (or any secret-typed staged value)
			// is masked in the review like every other secret value (#719).
			Secret: entry.ValueType == domain.ValueTypeSecret,
		}, nil

	case staging.OperationUpdate:
		if notFound {
			note, uerr := u.unstageGone(ctx, service, key, gone)
			if uerr != nil {
				return DiffEntry{}, uerr
			}

			return DiffEntry{
				Name:      key.Name,
				Namespace: key.Namespace,
				Type:      DiffEntryAutoUnstaged,
				Warning:   "item no longer exists in " + u.remoteLabel() + note,
			}, nil
		}
	}

	return DiffEntry{
		Name:      key.Name,
		Namespace: key.Namespace,
		Type:      DiffEntryWarning,
		Warning:   err.Error(),
	}, nil
}

// unstageGone unstages the entry and any staged tag changes of a key whose
// remote no longer exists, and records the key in gone. The tags must go too:
// left behind, they would fail the tag apply on the missing resource on every
// later apply until a manual reset (the reducer drops them for the same reason).
// It returns a note for the warning when staged tag changes were discarded.
//
//nolint:lll // function parameters are descriptive for clarity
func (u *DiffUseCase) unstageGone(ctx context.Context, service staging.Service, key staging.EntryKey, gone map[staging.EntryKey]struct{}) (string, error) {
	if err := u.Store.UnstageEntry(ctx, service, key); err != nil {
		return "", fmt.Errorf("failed to unstage %s: %w", key.Name, err)
	}

	gone[key] = struct{}{}

	switch err := u.Store.UnstageTag(ctx, service, key); {
	case errors.Is(err, staging.ErrNotStaged):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("failed to unstage tags of %s: %w", key.Name, err)
	default:
		return "; its staged tag changes were discarded", nil
	}
}

// discardVanishedTags probes the remote of a key that has staged tag changes but
// no staged entry. Only a genuine not-found unstages the tag changes (the same
// rule as unstageGone: left behind, they would fail every later apply) and
// returns an auto-unstaged row. Any other probe error keeps the tags staged; the
// tag row is shown as usual.
func (u *DiffUseCase) discardVanishedTags(ctx context.Context, key staging.EntryKey) (DiffEntry, bool, error) {
	strategy, err := u.strategyForNamespace(key.Namespace)
	if err != nil {
		return DiffEntry{}, false, err
	}

	if _, err := strategy.FetchCurrent(ctx, key.Name); !errors.Is(err, provider.ErrNotFound) {
		return DiffEntry{}, false, nil
	}

	if err := u.Store.UnstageTag(ctx, u.Strategy.Service(), key); err != nil {
		return DiffEntry{}, false, fmt.Errorf("failed to unstage tags of %s: %w", key.Name, err)
	}

	return DiffEntry{
		Name:      key.Name,
		Namespace: key.Namespace,
		Type:      DiffEntryAutoUnstaged,
		Warning:   "item no longer exists in " + u.remoteLabel() + "; its staged tag changes were discarded",
	}, true, nil
}
