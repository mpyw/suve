// Package status provides the global status command for viewing all staged changes.
package status

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
)

// Runner executes the status command.
type Runner struct {
	Store store.ReadWriteOperator
	// Services lists the provider services in stable display order.
	Services []stgcli.GlobalServiceSpec
	Stdout   io.Writer
	Stderr   io.Writer
}

// Options holds the options for the status command.
type Options struct {
	Verbose bool
}

// Command returns the status command for the given provider config.
func Command(cfg stgcli.GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show all staged changes",
		Description: `Display all staged changes for the active provider's services.

Use -v/--verbose to show detailed information including the staged values.

EXAMPLES:
   suve stage status     Show all staged changes
   suve stage status -v  Show detailed information`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, _, err := stgcli.WorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			r := &Runner{
				Store:    store,
				Services: cfg.Services,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			return r.Run(ctx, Options{Verbose: cmd.Bool("verbose")})
		},
	}
}

// Run executes the status command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	entries, err := r.Store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	tagEntries, err := r.Store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	if !hasAnyChanges(entries, tagEntries) {
		output.Info(r.Stdout, "No changes staged.")

		return nil
	}

	printer := &staging.EntryPrinter{Writer: r.Stdout}
	printed := false

	for _, spec := range r.Services {
		parser := spec.ParserFactory()

		svcEntries := entries[spec.Service]
		svcTags := tagEntries[spec.Service]

		total := len(svcEntries) + len(svcTags)
		if total == 0 {
			continue
		}

		if printed {
			output.Println(r.Stdout, "")
		}

		output.Printf(r.Stdout, "%s (%d):\n",
			colors.For(r.Stdout).Warning("Staged "+parser.ServiceName()+" changes"), total)
		printEntries(printer, svcEntries, opts.Verbose, parser.HasDeleteOptions())
		printTagEntries(r.Stdout, svcTags, opts.Verbose)

		printed = true
	}

	return nil
}

// hasAnyChanges reports whether any service has staged entries or tags.
func hasAnyChanges(entries map[staging.Service]map[string]staging.Entry, tagEntries map[staging.Service]map[string]staging.TagEntry) bool {
	for _, serviceEntries := range entries {
		if len(serviceEntries) > 0 {
			return true
		}
	}

	for _, serviceTags := range tagEntries {
		if len(serviceTags) > 0 {
			return true
		}
	}

	return false
}

func printEntries(printer *staging.EntryPrinter, entries map[string]staging.Entry, verbose, showDeleteOptions bool) {
	// Sort names for consistent output
	for _, name := range maputil.SortedKeys(entries) {
		printer.PrintEntry(name, entries[name], verbose, showDeleteOptions)
	}
}

func printTagEntries(w io.Writer, tagEntries map[string]staging.TagEntry, verbose bool) {
	for _, name := range maputil.SortedKeys(tagEntries) {
		entry := tagEntries[name]

		parts := []string{}
		if len(entry.Add) > 0 {
			parts = append(parts, fmt.Sprintf("+%d tag(s)", len(entry.Add)))
		}

		if entry.Remove.Len() > 0 {
			parts = append(parts, fmt.Sprintf("-%d tag(s)", entry.Remove.Len()))
		}

		summary := strings.Join(parts, ", ")
		output.Printf(w, "  %s %s [%s]\n", colors.For(w).Info("T"), name, summary)

		if verbose {
			for key, value := range entry.Add {
				output.Printf(w, "      + %s=%s\n", key, value)
			}

			for key := range entry.Remove {
				output.Printf(w, "      - %s\n", key)
			}
		}
	}
}
