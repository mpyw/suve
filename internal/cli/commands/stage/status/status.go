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
		Usage: "Show all staged changes (SSM Parameter Store and Secrets Manager)",
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

	// Show SSM Parameter Store changes (no DeleteOptions for SSM Parameter Store)
	if paramEntries, ok := entries[staging.ServiceParam]; ok && len(paramEntries) > 0 {
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning("Staged SSM Parameter Store changes"), len(paramEntries))
		printEntries(printer, paramEntries, opts.Verbose, false)
	}

	// Show Secrets Manager changes (with DeleteOptions)
	if secretEntries, ok := entries[staging.ServiceSecret]; ok && len(secretEntries) > 0 {
		// Add spacing if we printed SSM Parameter Store entries
		if _, ok := entries[staging.ServiceParam]; ok && len(entries[staging.ServiceParam]) > 0 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning("Staged Secrets Manager changes"), len(secretEntries))
		printEntries(printer, secretEntries, opts.Verbose, true)
	}

	return nil
}

func printEntries(printer *staging.EntryPrinter, entries map[string]staging.Entry, verbose, showDeleteOptions bool) {
	// Sort names for consistent output
	for _, name := range maputil.SortedKeys(entries) {
		printer.PrintEntry(name, entries[name], verbose, showDeleteOptions)
	}
}
