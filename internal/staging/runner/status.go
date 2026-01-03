// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StatusRunner executes status operations using a usecase.
type StatusRunner struct {
	UseCase *stagingusecase.StatusUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// StatusOptions holds options for the status command.
type StatusOptions struct {
	Name    string
	Verbose bool
}

// Run executes the status command.
func (r *StatusRunner) Run(ctx context.Context, opts StatusOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.StatusInput{
		Name: opts.Name,
	})
	if err != nil {
		return err
	}

	if len(result.Entries) == 0 {
		_, _ = fmt.Fprintf(r.Stdout, "No %s changes staged.\n", result.ServiceName)
		return nil
	}

	// For single item query, just print the entry
	if opts.Name != "" {
		printer := &staging.EntryPrinter{Writer: r.Stdout}
		for _, entry := range result.Entries {
			printer.PrintEntry(entry.Name, toStagingEntry(entry), opts.Verbose, entry.ShowDeleteOptions)
		}
		return nil
	}

	// For all items, show header and entries
	_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning(fmt.Sprintf("Staged %s changes", result.ServiceName)), len(result.Entries))

	printer := &staging.EntryPrinter{Writer: r.Stdout}
	for _, name := range r.sortedEntryNames(result.Entries) {
		for _, entry := range result.Entries {
			if entry.Name == name {
				printer.PrintEntry(entry.Name, toStagingEntry(entry), opts.Verbose, entry.ShowDeleteOptions)
				break
			}
		}
	}

	return nil
}

func (r *StatusRunner) sortedEntryNames(entries []stagingusecase.StatusEntry) []string {
	names := make(map[string]struct{})
	for _, e := range entries {
		names[e.Name] = struct{}{}
	}
	return maputil.SortedKeys(names)
}

func toStagingEntry(e stagingusecase.StatusEntry) staging.Entry {
	return staging.Entry{
		Operation:     e.Operation,
		Value:         e.Value,
		Description:   e.Description,
		Tags:          e.Tags,
		UntagKeys:     e.UntagKeys,
		DeleteOptions: e.DeleteOptions,
		StagedAt:      e.StagedAt,
	}
}
