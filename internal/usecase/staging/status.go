// Package staging provides use cases for staging operations.
package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
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
	Store    staging.StoreReader
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
		// Get specific entry
		entry, entryErr := u.Store.GetEntry(ctx, service, input.Name)
		if entryErr != nil && !errors.Is(entryErr, staging.ErrNotStaged) {
			return nil, entryErr
		}
		if entry != nil {
			output.Entries = []StatusEntry{toStatusEntry(input.Name, *entry, showDeleteOptions)}
		}

		// Get specific tag entry
		tagEntry, tagErr := u.Store.GetTag(ctx, service, input.Name)
		if tagErr != nil && !errors.Is(tagErr, staging.ErrNotStaged) {
			return nil, tagErr
		}
		if tagEntry != nil {
			output.TagEntries = []StatusTagEntry{toStatusTagEntry(input.Name, *tagEntry)}
		}

		// If neither exists, return error
		if entry == nil && tagEntry == nil {
			return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
		}

		return output, nil
	}

	// Get all entries
	entries, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}
	serviceEntries := entries[service]
	for name, entry := range serviceEntries {
		output.Entries = append(output.Entries, toStatusEntry(name, entry, showDeleteOptions))
	}

	// Get all tag entries
	tagEntries, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}
	serviceTagEntries := tagEntries[service]
	for name, tagEntry := range serviceTagEntries {
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
