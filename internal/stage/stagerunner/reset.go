// Package stageutil provides shared utilities for stage commands.
package stagerunner

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"

	"github.com/mpyw/suve/internal/stage"
)

// ResetRunner executes reset operations using a strategy.
type ResetRunner struct {
	Strategy stage.ResetStrategy
	Store    *stage.Store
	Stdout   io.Writer
	Stderr   io.Writer
}

// ResetOptions holds options for the reset command.
type ResetOptions struct {
	Spec string // Name with optional version spec
	All  bool   // Reset all staged items for this service
}

// Run executes the reset command.
func (r *ResetRunner) Run(ctx context.Context, opts ResetOptions) error {
	if opts.All {
		return r.runUnstageAll()
	}

	name, hasVersion, err := r.Strategy.ParseSpec(opts.Spec)
	if err != nil {
		return err
	}

	// If version specified, restore to that version
	if hasVersion {
		return r.runRestore(ctx, opts.Spec, name)
	}

	// Otherwise, just unstage
	return r.runUnstage(name)
}

func (r *ResetRunner) runUnstageAll() error {
	service := r.Strategy.Service()
	serviceName := r.Strategy.ServiceName()

	// Check if there are any staged changes
	staged, err := r.Store.List(service)
	if err != nil {
		return err
	}

	serviceStaged := staged[service]
	if len(serviceStaged) == 0 {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintf(r.Stdout, "%s\n", yellow(fmt.Sprintf("No %s changes staged.", serviceName)))
		return nil
	}

	if err := r.Store.UnstageAll(service); err != nil {
		return err
	}

	itemName := r.Strategy.ItemName()
	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all %s %ss (%d)\n", green("✓"), serviceName, itemName, len(serviceStaged))
	return nil
}

func (r *ResetRunner) runUnstage(name string) error {
	service := r.Strategy.Service()

	// Check if actually staged
	_, err := r.Store.Get(service, name)
	if err == stage.ErrNotStaged {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintf(r.Stdout, "%s %s is not staged\n", yellow("!"), name)
		return nil
	}
	if err != nil {
		return err
	}

	if err := r.Store.Unstage(service, name); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged %s\n", green("✓"), name)
	return nil
}

func (r *ResetRunner) runRestore(ctx context.Context, spec, name string) error {
	service := r.Strategy.Service()

	// Fetch the specific version
	value, versionLabel, err := r.Strategy.FetchVersion(ctx, spec)
	if err != nil {
		return err
	}

	// Stage this value
	if err := r.Store.Stage(service, name, stage.Entry{
		Operation: stage.OperationSet,
		Value:     value,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Restored %s (staged from version %s)\n",
		green("✓"), name, versionLabel)
	return nil
}
