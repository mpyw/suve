// Package status provides the global status command for viewing all staged changes.
package status

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
)

// Runner executes the status command.
type Runner struct {
	Store  *staging.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the status command.
type Options struct {
	Verbose bool
}

// Command returns the status command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show all staged changes (SSM and SM)",
		Description: `Display all staged changes for both SSM Parameter Store and Secrets Manager.

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
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := staging.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	opts := Options{
		Verbose: cmd.Bool("verbose"),
	}

	return r.Run(ctx, opts)
}

// Run executes the status command.
func (r *Runner) Run(_ context.Context, opts Options) error {
	entries, err := r.Store.List("")
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(r.Stdout, "No changes staged.")
		return nil
	}

	printer := &staging.EntryPrinter{Writer: r.Stdout}

	// Show SSM changes (no DeleteOptions for SSM)
	if ssmEntries, ok := entries[staging.ServiceSSM]; ok && len(ssmEntries) > 0 {
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning("Staged SSM changes"), len(ssmEntries))
		printEntries(printer, ssmEntries, opts.Verbose, false)
	}

	// Show SM changes (with DeleteOptions)
	if smEntries, ok := entries[staging.ServiceSM]; ok && len(smEntries) > 0 {
		// Add spacing if we printed SSM entries
		if _, ok := entries[staging.ServiceSSM]; ok && len(entries[staging.ServiceSSM]) > 0 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning("Staged SM changes"), len(smEntries))
		printEntries(printer, smEntries, opts.Verbose, true)
	}

	return nil
}

func printEntries(printer *staging.EntryPrinter, entries map[string]staging.Entry, verbose, showDeleteOptions bool) {
	// Sort names for consistent output
	for _, name := range maputil.SortedKeys(entries) {
		printer.PrintEntry(name, entries[name], verbose, showDeleteOptions)
	}
}
