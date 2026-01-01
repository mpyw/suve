// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"
	"time"

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
	Name            string // Optional: push only this item, otherwise push all
	IgnoreConflicts bool   // Skip conflict detection and force push
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

	// Check for conflicts unless --ignore-conflicts is specified
	if !opts.IgnoreConflicts {
		conflicts := r.checkConflicts(ctx, entries)
		if len(conflicts) > 0 {
			for _, name := range maputil.SortedKeys(conflicts) {
				output.Warning(r.Stderr, "conflict detected for %s: AWS was modified after staging", name)
			}
			return fmt.Errorf("push rejected: %d conflict(s) detected (use --ignore-conflicts to force)", len(conflicts))
		}
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

// checkConflicts checks if AWS resources were modified after staging.
// Returns a map of names that have conflicts.
func (r *PushRunner) checkConflicts(ctx context.Context, entries map[string]staging.Entry) map[string]struct{} {
	conflicts := make(map[string]struct{})

	// Only check Update and Delete operations (Create has nothing to conflict with)
	toCheck := make(map[string]staging.Entry)
	for name, entry := range entries {
		if entry.Operation == staging.OperationUpdate || entry.Operation == staging.OperationDelete {
			toCheck[name] = entry
		}
	}

	if len(toCheck) == 0 {
		return conflicts
	}

	// Fetch last modified times in parallel
	results := parallel.ExecuteMap(ctx, toCheck, func(ctx context.Context, name string, _ staging.Entry) (time.Time, error) {
		return r.Strategy.FetchLastModified(ctx, name)
	})

	// Check for conflicts
	for name, result := range results {
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on push anyway)
			continue
		}

		entry := toCheck[name]
		awsModified := result.Value

		// Zero time means resource doesn't exist - no conflict for delete (already gone)
		if awsModified.IsZero() {
			continue
		}

		// If AWS was modified after staging, it's a conflict
		if awsModified.After(entry.StagedAt) {
			conflicts[name] = struct{}{}
		}
	}

	return conflicts
}
