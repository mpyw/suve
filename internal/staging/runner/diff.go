// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// DiffRunner executes diff operations using a strategy.
type DiffRunner struct {
	Strategy staging.DiffStrategy
	Store    *staging.Store
	Stdout   io.Writer
	Stderr   io.Writer
}

// DiffOptions holds options for the diff command.
type DiffOptions struct {
	Name      string // Optional: diff only this item, otherwise diff all
	ParseJSON bool
	NoPager   bool
}

// Run executes the diff command.
func (r *DiffRunner) Run(ctx context.Context, opts DiffOptions) error {
	service := r.Strategy.Service()
	itemName := r.Strategy.ItemName()

	// Get all staged entries for the service
	allEntries, err := r.Store.List(service)
	if err != nil {
		return err
	}
	entries := allEntries[service]

	// Filter by name if specified
	if opts.Name != "" {
		entry, err := r.Store.Get(service, opts.Name)
		if errors.Is(err, staging.ErrNotStaged) {
			output.Warning(r.Stderr, "%s is not staged", opts.Name)
			return nil
		}
		if err != nil {
			return err
		}
		entries = map[string]staging.Entry{opts.Name: *entry}
	}

	if len(entries) == 0 {
		output.Warning(r.Stderr, "no %ss staged", itemName)
		return nil
	}

	// Fetch all values in parallel
	results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, name string, _ staging.Entry) (*staging.FetchResult, error) {
		return r.Strategy.FetchCurrent(ctx, name)
	})

	// Output results in sorted order
	first := true
	for _, name := range maputil.SortedKeys(entries) {
		entry := entries[name]
		result := results[name]

		if result.Err != nil {
			// Handle fetch error based on operation type
			switch entry.Operation {
			case staging.OperationDelete:
				// Item doesn't exist in AWS anymore - deletion already applied
				if err := r.Store.Unstage(service, name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}
				output.Warning(r.Stderr, "unstaged %s: already deleted in AWS", name)
				continue

			case staging.OperationCreate:
				// Item doesn't exist in AWS - this is expected for create operations
				// Show diff from empty to staged value
				if !first {
					_, _ = fmt.Fprintln(r.Stdout)
				}
				first = false
				if err := r.outputDiffCreate(opts, name, entry); err != nil {
					return err
				}
				continue

			case staging.OperationUpdate:
				// Item doesn't exist in AWS anymore - staged update is invalid
				if err := r.Store.Unstage(service, name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}
				output.Warning(r.Stderr, "unstaged %s: item no longer exists in AWS", name)
				continue
			}
		}

		if !first {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		first = false

		if err := r.outputDiff(opts, name, entry, result.Value); err != nil {
			return err
		}
	}

	return nil
}

func (r *DiffRunner) outputDiff(opts DiffOptions, name string, entry staging.Entry, fetchResult *staging.FetchResult) error {
	service := r.Strategy.Service()
	awsValue := fetchResult.Value
	stagedValue := lo.FromPtr(entry.Value)

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.ParseJSON {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, name)
	}

	if awsValue == stagedValue {
		// Auto-unstage since there's no difference
		if err := r.Store.Unstage(service, name); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}
		output.Warning(r.Stderr, "unstaged %s: identical to AWS current", name)
		return nil
	}

	label1 := fmt.Sprintf("%s%s (AWS)", name, fetchResult.Identifier)
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *DiffRunner) outputDiffCreate(opts DiffOptions, name string, entry staging.Entry) error {
	stagedValue := lo.FromPtr(entry.Value)

	// Format as JSON if enabled
	if opts.ParseJSON {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	label1 := fmt.Sprintf("%s (not in AWS)", name)
	label2 := fmt.Sprintf("%s (staged for creation)", name)

	diff := output.Diff(label1, label2, "", stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *DiffRunner) outputMetadata(entry staging.Entry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", colors.FieldLabel("Description:"), desc)
	}
	if len(entry.Tags) > 0 {
		var tagPairs []string
		for _, k := range maputil.SortedKeys(entry.Tags) {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, entry.Tags[k]))
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", colors.FieldLabel("Tags:"), strings.Join(tagPairs, ", "))
	}
}
