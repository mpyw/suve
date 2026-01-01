// Package status provides the global status command for viewing all staged changes.
package status

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/stage"
)

// Runner executes the status command.
type Runner struct {
	Store  *stage.Store
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
	store, err := stage.NewStore()
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
		_, _ = fmt.Fprintln(r.Stdout, "No staged changes.")
		return nil
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	printer := &stage.EntryPrinter{Writer: r.Stdout}

	// Show SSM changes (no DeleteOptions for SSM)
	if ssmEntries, ok := entries[stage.ServiceSSM]; ok && len(ssmEntries) > 0 {
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow("Staged SSM changes"), len(ssmEntries))
		printEntries(printer, ssmEntries, opts.Verbose, false)
	}

	// Show SM changes (with DeleteOptions)
	if smEntries, ok := entries[stage.ServiceSM]; ok && len(smEntries) > 0 {
		// Add spacing if we printed SSM entries
		if _, ok := entries[stage.ServiceSSM]; ok && len(entries[stage.ServiceSSM]) > 0 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow("Staged SM changes"), len(smEntries))
		printEntries(printer, smEntries, opts.Verbose, true)
	}

	return nil
}

func printEntries(printer *stage.EntryPrinter, entries map[string]stage.Entry, verbose, showDeleteOptions bool) {
	// Sort names for consistent output
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := entries[name]
		printer.PrintEntry(name, entry, verbose, showDeleteOptions)
	}
}
