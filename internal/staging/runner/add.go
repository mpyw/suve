// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/staging"
)

// AddRunner executes add operations using a strategy.
type AddRunner struct {
	Strategy   staging.Parser
	Store      *staging.Store
	Stdout     io.Writer
	Stderr     io.Writer
	OpenEditor editor.OpenFunc // Optional: defaults to editor.Open if nil
}

// AddOptions holds options for the add command.
type AddOptions struct {
	Name        string
	Value       string // Optional: if set, skip editor and use this value
	Description string
	Tags        map[string]string
}

// Run executes the add command.
func (r *AddRunner) Run(_ context.Context, opts AddOptions) error {
	service := r.Strategy.Service()

	// Parse and validate name
	name, err := r.Strategy.ParseName(opts.Name)
	if err != nil {
		return err
	}

	// Check if already staged
	stagedEntry, err := r.Store.Get(service, name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return err
	}

	var currentValue string
	if stagedEntry != nil && stagedEntry.Operation == staging.OperationCreate {
		// Already staged as create, allow editing
		currentValue = lo.FromPtr(stagedEntry.Value)
	}
	// For new items, currentValue stays empty

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

		// Check if value is empty (canceled)
		if newValue == "" {
			_, _ = fmt.Fprintln(r.Stdout, colors.Warning("Empty value, not staged."))
			return nil
		}

		// Check if unchanged from staged value
		if stagedEntry != nil && newValue == currentValue {
			_, _ = fmt.Fprintln(r.Stdout, colors.Warning("No changes made."))
			return nil
		}
	}

	// Stage the change with OperationCreate
	entry := staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(newValue),
		StagedAt:  time.Now(),
	}
	if opts.Description != "" {
		entry.Description = &opts.Description
	}
	if len(opts.Tags) > 0 {
		entry.Tags = opts.Tags
	}
	if err := r.Store.Stage(service, name, entry); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Staged for creation: %s\n", colors.Success("✓"), name)
	return nil
}
