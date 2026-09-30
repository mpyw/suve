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

// statusRunner executes status operations using a usecase.
//
//declscope:package // command.go and global_status.go build and run it
type statusRunner struct {
	useCase *stagingusecase.StatusUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// statusOptions holds options for the status command.
//
//declscope:package // command.go fills it from the flags
type statusOptions struct {
	name    string
	verbose bool
}

// run executes the status command.
//
//declscope:package // command.go runs it
func (r *statusRunner) run(ctx context.Context, opts statusOptions) error {
	result, err := r.useCase.Execute(ctx, stagingusecase.StatusInput{
		Name: opts.name,
	})
	if err != nil {
		return err
	}

	totalCount := len(result.Entries) + len(result.TagEntries)
	if totalCount == 0 {
		output.Info(r.stdout, "No %s changes staged.", result.ServiceName)

		return nil
	}

	// For single item query, just print the entry
	if opts.name != "" {
		printer := &staging.EntryPrinter{Writer: r.stdout}
		for _, entry := range result.Entries {
			key := staging.EntryKey{Name: entry.Name, Namespace: entry.Namespace}
			printer.PrintEntry(key, stagingEntryFromStatus(entry), opts.verbose, entry.ShowDeleteOptions)
		}

		for _, tagEntry := range result.TagEntries {
			r.printTagEntry(tagEntry, opts.verbose)
		}

		return nil
	}

	r.printService(result, opts.verbose)

	return nil
}

// printService prints one service's staged changes under a
// "Staged <service> changes (N):" header.
//
//declscope:package // the all-service status (global_status.go) renders through it
func (r *statusRunner) printService(result *stagingusecase.StatusOutput, verbose bool) {
	output.Printf(r.stdout, "%s (%d):\n",
		colors.For(r.stdout).Warning(fmt.Sprintf("Staged %s changes", result.ServiceName)),
		len(result.Entries)+len(result.TagEntries))

	printer := &staging.EntryPrinter{Writer: r.stdout}

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
		key := staging.EntryKey{Name: entry.Name, Namespace: entry.Namespace}
		printer.PrintEntry(key, stagingEntryFromStatus(entry), verbose, entry.ShowDeleteOptions)
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
		r.printTagEntry(tagEntry, verbose)
	}
}

func (r *statusRunner) printTagEntry(e stagingusecase.StatusTagEntry, verbose bool) {
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
		nameLabel += " " + colors.For(r.stdout).FieldLabel("["+e.Namespace+"]")
	}

	output.Printf(r.stdout, "  %s %s [%s]\n", colors.For(r.stdout).Info("T"), nameLabel, summary)

	if verbose {
		for key, value := range e.Add {
			output.Printf(r.stdout, "      + %s=%s\n", key, value)
		}

		for key := range e.Remove {
			output.Printf(r.stdout, "      - %s\n", key)
		}
	}
}

func stagingEntryFromStatus(e stagingusecase.StatusEntry) staging.Entry {
	return staging.Entry{
		Operation:     e.Operation,
		Value:         e.Value,
		Description:   e.Description,
		DeleteOptions: e.DeleteOptions,
		StagedAt:      e.StagedAt,
	}
}
