// Package stageutil provides shared utilities for stage commands.
package stageutil

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"

	"github.com/mpyw/suve/internal/editor"
	"github.com/mpyw/suve/internal/stage"
)

// EditRunner executes edit operations using a strategy.
type EditRunner struct {
	Strategy   stage.EditStrategy
	Store      *stage.Store
	Stdout     io.Writer
	Stderr     io.Writer
	OpenEditor editor.OpenFunc // Optional: defaults to editor.Open if nil
}

// EditOptions holds options for the edit command.
type EditOptions struct {
	Name string
}

// Run executes the edit command.
func (r *EditRunner) Run(ctx context.Context, opts EditOptions) error {
	service := r.Strategy.Service()

	// Check if already staged
	stagedEntry, err := r.Store.Get(service, opts.Name)
	if err != nil && err != stage.ErrNotStaged {
		return err
	}

	var currentValue string
	if stagedEntry != nil && stagedEntry.Operation == stage.OperationSet {
		// Use staged value
		currentValue = stagedEntry.Value
	} else {
		// Fetch from AWS
		currentValue, err = r.Strategy.FetchCurrentValue(ctx, opts.Name)
		if err != nil {
			return err
		}
	}

	// Open editor
	editorFn := r.OpenEditor
	if editorFn == nil {
		editorFn = editor.Open
	}
	newValue, err := editorFn(currentValue)
	if err != nil {
		return fmt.Errorf("failed to edit: %w", err)
	}

	// Check if changed
	if newValue == currentValue {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No changes made."))
		return nil
	}

	// Stage the change
	if err := r.Store.Stage(service, opts.Name, stage.Entry{
		Operation: stage.OperationSet,
		Value:     newValue,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Staged: %s\n", green("✓"), opts.Name)
	return nil
}
