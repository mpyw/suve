// Package status provides the global status command for viewing all staged changes.
package status

import (
	"context"
	"errors"
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
	// Store, when set, is used for every service (a test seam). When nil each
	// service resolves its own working store via its spec's ScopeResolver — Azure
	// App Configuration and Key Vault live in separate staging buckets.
	Store store.ReadWriteOperator
	// Services lists the provider services in stable display order.
	Services []stgcli.GlobalServiceSpec
	Stdout   io.Writer
	Stderr   io.Writer
}

// storeForService returns the injected Store (test seam) or resolves this
// service's own working store.
func (r *Runner) storeForService(ctx context.Context, spec stgcli.GlobalServiceSpec) (store.ReadWriteOperator, error) {
	if r.Store != nil {
		return r.Store, nil
	}

	st, _, err := stgcli.WorkingStore(ctx, spec.ScopeResolver)

	return st, err
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
			r := &Runner{
				Services: cfg.Services,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			return r.Run(ctx, Options{Verbose: cmd.Bool("verbose")})
		},
	}
}

// Run executes the status command. Each service reads its OWN store; a service
// whose scope is not configured (e.g. no Key Vault while only App Configuration
// is set) is skipped — an unconfigured service can hold no staged state.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	printer := &staging.EntryPrinter{Writer: r.Stdout}
	printed := false

	for _, spec := range r.Services {
		st, err := r.storeForService(ctx, spec)
		if errors.Is(err, staging.ErrServiceNotConfigured) {
			continue
		}

		if err != nil {
			return err
		}

		entries, err := st.ListEntries(ctx, spec.Service)
		if err != nil {
			return err
		}

		tagEntries, err := st.ListTags(ctx, spec.Service)
		if err != nil {
			return err
		}

		svcEntries := entries[spec.Service]
		svcTags := tagEntries[spec.Service]

		total := len(svcEntries) + len(svcTags)
		if total == 0 {
			continue
		}

		parser := spec.ParserFactory()

		if printed {
			output.Println(r.Stdout, "")
		}

		output.Printf(r.Stdout, "%s (%d):\n",
			colors.For(r.Stdout).Warning("Staged "+parser.ServiceName()+" changes"), total)
		printEntries(printer, svcEntries, opts.Verbose, parser.HasDeleteOptions())
		printTagEntries(r.Stdout, svcTags, opts.Verbose)

		printed = true
	}

	if !printed {
		output.Info(r.Stdout, "No changes staged.")
	}

	return nil
}

func printEntries(printer *staging.EntryPrinter, entries map[string]staging.Entry, verbose, showDeleteOptions bool) {
	// Entries are keyed by the (name, namespace) composite; print each under its
	// decoded bare name (the printer badges the namespace from the entry).
	for _, key := range maputil.SortedKeys(entries) {
		name, _ := staging.SplitEntryKey(key)
		printer.PrintEntry(name, entries[key], verbose, showDeleteOptions)
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
