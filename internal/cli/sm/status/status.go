// Package status provides the SM status command for viewing staged changes.
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
		Usage:     "Show staged secret changes",
		ArgsUsage: "[name]",
		Description: `Display staged changes for AWS Secrets Manager.

Without arguments, shows all staged secret changes.
With a secret name, shows the staged change for that specific secret.

Use -v/--verbose to show detailed information including the staged value.

EXAMPLES:
   suve sm status             Show all staged SM changes
   suve sm status my-secret   Show staged change for specific secret
   suve sm status -v          Show detailed information`,
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
	entry, err := r.Store.Get(stage.ServiceSM, name)
	if err != nil {
		if err == stage.ErrNotStaged {
			return fmt.Errorf("secret %s is not staged", name)
		}
		return err
	}

	r.printEntry(name, *entry, verbose)
	return nil
}

func (r *Runner) showAll(verbose bool) error {
	entries, err := r.Store.List(stage.ServiceSM)
	if err != nil {
		return err
	}

	smEntries := entries[stage.ServiceSM]
	if len(smEntries) == 0 {
		_, _ = fmt.Fprintln(r.Stdout, "No staged SM changes.")
		return nil
	}

	// Sort names for consistent output
	names := make([]string, 0, len(smEntries))
	for name := range smEntries {
		names = append(names, name)
	}
	sort.Strings(names)

	yellow := color.New(color.FgYellow).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow("Staged SM changes"), len(smEntries))

	for _, name := range names {
		entry := smEntries[name]
		r.printEntry(name, entry, verbose)
	}

	return nil
}

func (r *Runner) printEntry(name string, entry stage.Entry, verbose bool) {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	var opSymbol, opColor string
	switch entry.Operation {
	case stage.OperationSet:
		opSymbol = "M"
		opColor = green(opSymbol)
	case stage.OperationDelete:
		opSymbol = "D"
		opColor = red(opSymbol)
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
