// Package stageutil provides shared utilities for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// PushRunner executes push operations using a strategy.
type PushRunner struct {
	Strategy staging.PushStrategy
	Store    *staging.Store
	Stdout   io.Writer
	Stderr   io.Writer
}

// PushOptions holds options for the push command.
type PushOptions struct {
	Name string // Optional: push only this item, otherwise push all
}

// Run executes the push command.
func (r *PushRunner) Run(ctx context.Context, opts PushOptions) error {
	service := r.Strategy.Service()
	itemName := r.Strategy.ItemName()

	staged, err := r.Store.List(service)
	if err != nil {
		return err
	}

	entries := staged[service]
	if len(entries) == 0 {
		output.Info(r.Stdout, "No %s changes staged.", r.Strategy.ServiceName())
		return nil
	}

	// Filter by name if specified
	if opts.Name != "" {
		entry, exists := entries[opts.Name]
		if !exists {
			return fmt.Errorf("%s %s is not staged", itemName, opts.Name)
		}
		entries = map[string]staging.Entry{opts.Name: entry}
	}

	// Execute push operations in parallel
	results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, name string, entry staging.Entry) (staging.Operation, error) {
		err := r.Strategy.Push(ctx, name, entry)
		return entry.Operation, err
	})

	// Output results in sorted order
	var succeeded, failed int
	for _, name := range maputil.SortedKeys(entries) {
		result := results[name]
		if result.Err != nil {
			output.Failed(r.Stderr, name, result.Err)
			failed++
		} else {
			switch result.Value {
			case staging.OperationCreate:
				output.Success(r.Stdout, "Created %s", name)
			case staging.OperationUpdate:
				output.Success(r.Stdout, "Updated %s", name)
			case staging.OperationDelete:
				output.Success(r.Stdout, "Deleted %s", name)
			}
			if err := r.Store.Unstage(service, name); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", name, err)
			}
			succeeded++
		}
	}

	if failed > 0 {
		return fmt.Errorf("pushed %d, failed %d", succeeded, failed)
	}

	return nil
}
