package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
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
		output.Info(r.Stdout, "No %s changes staged.", result.ServiceName)

		return nil
	}

	// For single item query, just print the entry
	if opts.Name != "" {
		printer := &staging.EntryPrinter{Writer: r.Stdout}
		for _, entry := range result.Entries {
			printer.PrintEntry(staging.EntryKey{Name: entry.Name, Namespace: entry.Namespace}, toStagingEntry(entry), opts.Verbose, entry.ShowDeleteOptions)
		}

		for _, tagEntry := range result.TagEntries {
			r.printTagEntry(tagEntry, opts.Verbose)
		}

		return nil
	}

	// For all items, show header and entries
	output.Printf(r.Stdout, "%s (%d):\n", colors.For(r.Stdout).Warning(fmt.Sprintf("Staged %s changes", result.ServiceName)), totalCount)

	printer := &staging.EntryPrinter{Writer: r.Stdout}

	// Sort by (name, namespace): the same App Configuration key staged under
	// several namespaces is several distinct entries, so we must print each one
	// (deduping by name would drop all but one, order-dependently).
	entries := slices.Clone(result.Entries)
	slices.SortFunc(entries, func(a, b stagingusecase.StatusEntry) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return cmp.Compare(a.Namespace, b.Namespace)
	})

	for _, entry := range entries {
		printer.PrintEntry(staging.EntryKey{Name: entry.Name, Namespace: entry.Namespace}, toStagingEntry(entry), opts.Verbose, entry.ShowDeleteOptions)
	}

	// Print tag entries, sorted by (name, namespace). Like entries, the same App
	// Configuration key tagged under several namespaces is several distinct tag
	// entries — deduping by name would drop all but one.
	tagEntries := slices.Clone(result.TagEntries)
	slices.SortFunc(tagEntries, func(a, b stagingusecase.StatusTagEntry) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return cmp.Compare(a.Namespace, b.Namespace)
	})

	for _, tagEntry := range tagEntries {
		r.printTagEntry(tagEntry, opts.Verbose)
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

	// App Configuration tags carry a namespace (the label axis); badge it inline
	// so the same key tagged under several namespaces is unambiguous.
	nameLabel := e.Name
	if e.Namespace != "" {
		nameLabel += " " + colors.For(r.Stdout).FieldLabel("["+e.Namespace+"]")
	}

	output.Printf(r.Stdout, "  %s %s [%s]\n", colors.For(r.Stdout).Info("T"), nameLabel, summary)

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
