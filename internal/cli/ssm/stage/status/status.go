// Package status provides the SSM status command for viewing staged changes.
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
	Name    string
	Verbose bool
}

// Command returns the status command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show staged parameter changes",
		ArgsUsage: "[name]",
		Description: `Display staged changes for SSM Parameter Store.

Without arguments, shows all staged SSM parameter changes.
With a parameter name, shows the staged change for that specific parameter.

Use -v/--verbose to show detailed information including the staged value.

EXAMPLES:
   suve ssm stage status              Show all staged SSM changes
   suve ssm stage status /app/config  Show staged change for specific parameter
   suve ssm stage status -v           Show detailed information`,
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
	if cmd.Args().Len() > 0 {
		opts.Name = cmd.Args().First()
	}

	return r.Run(ctx, opts)
}

// Run executes the status command.
func (r *Runner) Run(_ context.Context, opts Options) error {
	if opts.Name != "" {
		return r.showSingle(opts.Name, opts.Verbose)
	}
	return r.showAll(opts.Verbose)
}

func (r *Runner) showSingle(name string, verbose bool) error {
	entry, err := r.Store.Get(stage.ServiceSSM, name)
	if err != nil {
		if err == stage.ErrNotStaged {
			return fmt.Errorf("parameter %s is not staged", name)
		}
		return err
	}

	printer := &stage.EntryPrinter{Writer: r.Stdout}
	printer.PrintEntry(name, *entry, verbose, false) // SSM has no DeleteOptions
	return nil
}

func (r *Runner) showAll(verbose bool) error {
	entries, err := r.Store.List(stage.ServiceSSM)
	if err != nil {
		return err
	}

	ssmEntries := entries[stage.ServiceSSM]
	if len(ssmEntries) == 0 {
		_, _ = fmt.Fprintln(r.Stdout, "No SSM changes staged.")
		return nil
	}

	// Sort names for consistent output
	names := make([]string, 0, len(ssmEntries))
	for name := range ssmEntries {
		names = append(names, name)
	}
	sort.Strings(names)

	yellow := color.New(color.FgYellow).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow("Staged SSM changes"), len(ssmEntries))

	printer := &stage.EntryPrinter{Writer: r.Stdout}
	for _, name := range names {
		entry := ssmEntries[name]
		printer.PrintEntry(name, entry, verbose, false) // SSM has no DeleteOptions
	}

	return nil
}
