package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
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

	totalCount := len(result.Entries) + len(result.TagEntries)
	if totalCount == 0 {
		output.Printf(r.Stdout, "No %s changes staged.\n", result.ServiceName)
		return nil
	}

	// For single item query, just print the entry
	if opts.Name != "" {
		printer := &staging.EntryPrinter{Writer: r.Stdout}
		for _, entry := range result.Entries {
			printer.PrintEntry(entry.Name, toStagingEntry(entry), opts.Verbose, entry.ShowDeleteOptions)
		}

		for _, tagEntry := range result.TagEntries {
			r.printTagEntry(tagEntry, opts.Verbose)
		}
		return nil
	}

	// For all items, show header and entries
	output.Printf(r.Stdout, "%s (%d):\n", colors.Warning(fmt.Sprintf("Staged %s changes", result.ServiceName)), totalCount)

	printer := &staging.EntryPrinter{Writer: r.Stdout}

	for _, name := range maputil.SortedNames(result.Entries, func(e stagingusecase.StatusEntry) string { return e.Name }) {
		for _, entry := range result.Entries {
			if entry.Name == name {
				printer.PrintEntry(entry.Name, toStagingEntry(entry), opts.Verbose, entry.ShowDeleteOptions)

				break
			}
		}
	}

	// Print tag entries
	for _, name := range maputil.SortedNames(result.TagEntries, func(e stagingusecase.StatusTagEntry) string { return e.Name }) {
		for _, tagEntry := range result.TagEntries {
			if tagEntry.Name == name {
				r.printTagEntry(tagEntry, opts.Verbose)

				break
			}
		}
	}

	return nil
}

func (r *StatusRunner) printTagEntry(e stagingusecase.StatusTagEntry, verbose bool) {
	parts := []string{}
	if len(e.Add) > 0 {
		parts = append(parts, fmt.Sprintf("+%d tag(s)", len(e.Add)))
	}

	if e.Remove.Len() > 0 {
		parts = append(parts, fmt.Sprintf("-%d tag(s)", e.Remove.Len()))
	}

	summary := strings.Join(parts, ", ")
	output.Printf(r.Stdout, "  %s %s [%s]\n", colors.Info("T"), e.Name, summary)

	if verbose {
		for key, value := range e.Add {
			output.Printf(r.Stdout, "      + %s=%s\n", key, value)
		}

		for key := range e.Remove {
			output.Printf(r.Stdout, "      - %s\n", key)
		}
	}
}

func toStagingEntry(e stagingusecase.StatusEntry) staging.Entry {
	return staging.Entry{
		Operation:     e.Operation,
		Value:         e.Value,
		Description:   e.Description,
		DeleteOptions: e.DeleteOptions,
		StagedAt:      e.StagedAt,
	}
}
