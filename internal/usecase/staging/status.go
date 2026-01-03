// Package staging provides use cases for staging operations.
package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/staging"
)

// StatusInput holds input for the status use case.
type StatusInput struct {
	Name string // Optional: if set, show only this item
}

// StatusEntry represents a single staged entry.
type StatusEntry struct {
	Name              string
	Operation         staging.Operation
	Value             *string
	Description       *string
	Tags              map[string]string
	UntagKeys         []string
	DeleteOptions     *staging.DeleteOptions
	StagedAt          time.Time
	ShowDeleteOptions bool
}

// StatusOutput holds the result of the status use case.
type StatusOutput struct {
	Service     staging.Service
	ServiceName string
	ItemName    string
	Entries     []StatusEntry
}

// StatusUseCase executes status operations.
type StatusUseCase struct {
	Strategy staging.ServiceStrategy
	Store    staging.StoreReader
}

// Execute runs the status use case.
func (u *StatusUseCase) Execute(_ context.Context, input StatusInput) (*StatusOutput, error) {
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
		entry, err := u.Store.Get(service, input.Name)
		if err != nil {
			if errors.Is(err, staging.ErrNotStaged) {
				return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
			}
			return nil, err
		}
		output.Entries = []StatusEntry{
			toStatusEntry(input.Name, *entry, showDeleteOptions),
		}
		return output, nil
	}

	entries, err := u.Store.List(service)
	if err != nil {
		return nil, err
	}

	serviceEntries := entries[service]
	for name, entry := range serviceEntries {
		output.Entries = append(output.Entries, toStatusEntry(name, entry, showDeleteOptions))
	}

	return output, nil
}

func toStatusEntry(name string, entry staging.Entry, showDeleteOptions bool) StatusEntry {
	return StatusEntry{
		Name:              name,
		Operation:         entry.Operation,
		Value:             entry.Value,
		Description:       entry.Description,
		Tags:              entry.Tags,
		UntagKeys:         entry.UntagKeys,
		DeleteOptions:     entry.DeleteOptions,
		StagedAt:          entry.StagedAt,
		ShowDeleteOptions: showDeleteOptions,
	}
}
