// Package stageutil provides shared utilities for stage commands.
package stageutil

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/fatih/color"

	"github.com/mpyw/suve/internal/stage"
)

// StatusRunner executes status operations using a strategy.
type StatusRunner struct {
	Strategy stage.ServiceStrategy
	Store    *stage.Store
	Stdout   io.Writer
	Stderr   io.Writer
}

// StatusOptions holds options for the status command.
type StatusOptions struct {
	Name    string
	Verbose bool
}

// Run executes the status command.
func (r *StatusRunner) Run(_ context.Context, opts StatusOptions) error {
	if opts.Name != "" {
		return r.showSingle(opts.Name, opts.Verbose)
	}
	return r.showAll(opts.Verbose)
}

func (r *StatusRunner) showSingle(name string, verbose bool) error {
	service := r.Strategy.Service()
	itemName := r.Strategy.ItemName()
	showDeleteOptions := r.Strategy.HasDeleteOptions()

	entry, err := r.Store.Get(service, name)
	if err != nil {
		if err == stage.ErrNotStaged {
			return fmt.Errorf("%s %s is not staged", itemName, name)
		}
		return err
	}

	printer := &stage.EntryPrinter{Writer: r.Stdout}
	printer.PrintEntry(name, *entry, verbose, showDeleteOptions)
	return nil
}

func (r *StatusRunner) showAll(verbose bool) error {
	service := r.Strategy.Service()
	serviceName := r.Strategy.ServiceName()
	showDeleteOptions := r.Strategy.HasDeleteOptions()

	entries, err := r.Store.List(service)
	if err != nil {
		return err
	}

	serviceEntries := entries[service]
	if len(serviceEntries) == 0 {
		_, _ = fmt.Fprintf(r.Stdout, "No %s changes staged.\n", serviceName)
		return nil
	}

	// Sort names for consistent output
	names := make([]string, 0, len(serviceEntries))
	for name := range serviceEntries {
		names = append(names, name)
	}
	sort.Strings(names)

	yellow := color.New(color.FgYellow).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", yellow(fmt.Sprintf("Staged %s changes", serviceName)), len(serviceEntries))

	printer := &stage.EntryPrinter{Writer: r.Stdout}
	for _, name := range names {
		entry := serviceEntries[name]
		printer.PrintEntry(name, entry, verbose, showDeleteOptions)
	}

	return nil
}
