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
	Name              string
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
	Store    store.ServiceLister
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

	if input.Name != "" {
		// Get specific entry by scanning the list (ServiceLister doesn't have GetEntry)
		entries, err := u.Store.ListEntries(ctx)
		if err != nil {
			return nil, err
		}

		if entry, exists := entries[input.Name]; exists {
			output.Entries = []StatusEntry{toStatusEntry(input.Name, entry, showDeleteOptions)}
		}

		// Get specific tag entry by scanning the list
		tagEntries, err := u.Store.ListTags(ctx)
		if err != nil {
			return nil, err
		}

		if tagEntry, exists := tagEntries[input.Name]; exists {
			output.TagEntries = []StatusTagEntry{toStatusTagEntry(input.Name, tagEntry)}
		}

		// If neither exists, return error
		if len(output.Entries) == 0 && len(output.TagEntries) == 0 {
			return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
		}

		return output, nil
	}

	// Get all entries
	entries, err := u.Store.ListEntries(ctx)
	if err != nil {
		return nil, err
	}

	for name, entry := range entries {
		output.Entries = append(output.Entries, toStatusEntry(name, entry, showDeleteOptions))
	}

	// Get all tag entries
	tagEntries, err := u.Store.ListTags(ctx)
	if err != nil {
		return nil, err
	}

	for name, tagEntry := range tagEntries {
		output.TagEntries = append(output.TagEntries, toStatusTagEntry(name, tagEntry))
	}

	return output, nil
}

func toStatusEntry(name string, entry staging.Entry, showDeleteOptions bool) StatusEntry {
	return StatusEntry{
		Name:              name,
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
