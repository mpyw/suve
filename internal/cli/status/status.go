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
   suve status     Show all staged changes
   suve status -v  Show detailed information`,
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

	// Show SSM changes
	if ssmEntries, ok := entries[stage.ServiceSSM]; ok && len(ssmEntries) > 0 {
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow("Staged SSM changes"), len(ssmEntries))
		r.printEntries(ssmEntries, opts.Verbose)
	}

	// Show SM changes
	if smEntries, ok := entries[stage.ServiceSM]; ok && len(smEntries) > 0 {
		// Add spacing if we printed SSM entries
		if _, ok := entries[stage.ServiceSSM]; ok && len(entries[stage.ServiceSSM]) > 0 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow("Staged SM changes"), len(smEntries))
		r.printEntries(smEntries, opts.Verbose)
	}

	return nil
}

func (r *Runner) printEntries(entries map[string]stage.Entry, verbose bool) {
	// Sort names for consistent output
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := entries[name]
		r.printEntry(name, entry, verbose)
	}
}

func (r *Runner) printEntry(name string, entry stage.Entry, verbose bool) {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	var opColor string
	switch entry.Operation {
	case stage.OperationSet:
		opColor = green("M")
	case stage.OperationDelete:
		opColor = red("D")
	}

	if verbose {
		_, _ = fmt.Fprintf(r.Stdout, "\n%s %s\n", opColor, name)
		_, _ = fmt.Fprintf(r.Stdout, "  %s %s\n", cyan("Staged:"), entry.StagedAt.Format("2006-01-02 15:04:05"))
		if entry.Operation == stage.OperationSet {
			value := entry.Value
			if len(value) > 100 {
				value = value[:100] + "..."
			}
			_, _ = fmt.Fprintf(r.Stdout, "  %s %s\n", cyan("Value:"), value)
		}
	} else {
		_, _ = fmt.Fprintf(r.Stdout, "  %s %s\n", opColor, name)
	}
}
