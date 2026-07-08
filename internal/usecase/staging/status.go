// Package staging provides use cases for staging operations.
package staging

import (
	"context"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// StatusInput holds input for the status use case.
type StatusInput struct {
	Name string // Optional: if set, show only this item
}

// StatusEntry represents a single staged entry (create/update/delete).
type StatusEntry struct {
	Name string
	// Namespace is the App Configuration namespace of the entry (empty for the
	// null/default namespace and every other provider).
	Namespace         string
	Operation         staging.Operation
	Value             *string
	Description       *string
	DeleteOptions     *staging.DeleteOptions
	StagedAt          time.Time
	ShowDeleteOptions bool
}

// StatusTagEntry represents staged tag changes for an entity.
type StatusTagEntry struct {
	Name     string
	Add      map[string]string   // Tags to add or update
	Remove   maputil.Set[string] // Tag keys to remove
	StagedAt time.Time
}

// StatusOutput holds the result of the status use case.
type StatusOutput struct {
	Service     staging.Service
	ServiceName string
	ItemName    string
	Entries     []StatusEntry
	TagEntries  []StatusTagEntry
}

// StatusUseCase executes status operations.
type StatusUseCase struct {
	Strategy staging.ServiceStrategy
	Store    store.ReadOperator
}

// Execute runs the status use case.
func (u *StatusUseCase) Execute(ctx context.Context, input StatusInput) (*StatusOutput, error) {
	service := u.Strategy.Service()
	serviceName := u.Strategy.ServiceName()
	itemName := u.Strategy.ItemName()
	showDeleteOptions := u.Strategy.HasDeleteOptions()

	output := &StatusOutput{
		Service:     service,
		ServiceName: serviceName,
		ItemName:    itemName,
	}

	// Get all entries and tags, then (if a name was given) filter by the decoded
	// bare name. Entries are keyed by the (name, namespace) composite, so a name
	// with settings under several namespaces yields one row per namespace.
	entries, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}

	tagEntries, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}

	matched := false

	for key, entry := range entries[service] {
		name, _ := staging.SplitEntryKey(key)
		if input.Name != "" && name != input.Name {
			continue
		}

		output.Entries = append(output.Entries, toStatusEntry(name, entry, showDeleteOptions))
		matched = true
	}

	for key, tagEntry := range tagEntries[service] {
		name, _ := staging.SplitEntryKey(key)
		if input.Name != "" && name != input.Name {
			continue
		}

		output.TagEntries = append(output.TagEntries, toStatusTagEntry(name, tagEntry))
		matched = true
	}

	if input.Name != "" && !matched {
		return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
	}

	return output, nil
}

func toStatusEntry(name string, entry staging.Entry, showDeleteOptions bool) StatusEntry {
	return StatusEntry{
		Name:              name,
		Namespace:         entry.Namespace,
		Operation:         entry.Operation,
		Value:             entry.Value,
		Description:       entry.Description,
		DeleteOptions:     entry.DeleteOptions,
		StagedAt:          entry.StagedAt,
		ShowDeleteOptions: showDeleteOptions,
	}
}

func toStatusTagEntry(name string, tagEntry staging.TagEntry) StatusTagEntry {
	return StatusTagEntry{
		Name:     name,
		Add:      tagEntry.Add,
		Remove:   tagEntry.Remove,
		StagedAt: tagEntry.StagedAt,
	}
}
