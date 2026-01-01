// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"

	"github.com/mpyw/suve/internal/editor"
	"github.com/mpyw/suve/internal/staging"
)

// EditRunner executes edit operations using a strategy.
type EditRunner struct {
	Strategy   staging.EditStrategy
	Store      *staging.Store
	Stdout     io.Writer
	Stderr     io.Writer
	OpenEditor editor.OpenFunc // Optional: defaults to editor.Open if nil
}

// EditOptions holds options for the edit command.
type EditOptions struct {
	Name        string
	Value       string // Optional: if set, skip editor and use this value
	Description string
	Tags        map[string]string
}

// Run executes the edit command.
func (r *EditRunner) Run(ctx context.Context, opts EditOptions) error {
	service := r.Strategy.Service()

	// Check if already staged
	stagedEntry, err := r.Store.Get(service, opts.Name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return err
	}

	var currentValue string
	if stagedEntry != nil && (stagedEntry.Operation == staging.OperationCreate || stagedEntry.Operation == staging.OperationUpdate) {
		// Use staged value
		currentValue = stagedEntry.Value
	} else {
		// Fetch from AWS
		currentValue, err = r.Strategy.FetchCurrentValue(ctx, opts.Name)
		if err != nil {
			return err
		}
	}

	var newValue string
	if opts.Value != "" {
		// Use provided value, skip editor
		newValue = opts.Value
	} else {
		// Open editor
		editorFn := r.OpenEditor
		if editorFn == nil {
			editorFn = editor.Open
		}
		newValue, err = editorFn(currentValue)
		if err != nil {
			return fmt.Errorf("failed to edit: %w", err)
		}

		// Check if changed
		if newValue == currentValue {
			yellow := color.New(color.FgYellow).SprintFunc()
			_, _ = fmt.Fprintln(r.Stdout, yellow("No changes made."))
			return nil
		}
	}

	// Stage the change
	entry := staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     newValue,
		StagedAt:  time.Now(),
	}
	if opts.Description != "" {
		entry.Description = &opts.Description
	}
	if len(opts.Tags) > 0 {
		entry.Tags = opts.Tags
	}
	if err := r.Store.Stage(service, opts.Name, entry); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Staged: %s\n", green("✓"), opts.Name)
	return nil
}
