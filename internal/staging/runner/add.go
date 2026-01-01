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
	Name string
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
		currentValue = stagedEntry.Value
	}
	// For new items, currentValue stays empty

	// Open editor
	editorFn := r.OpenEditor
	if editorFn == nil {
		editorFn = editor.Open
	}
	newValue, err := editorFn(currentValue)
	if err != nil {
		return fmt.Errorf("failed to edit: %w", err)
	}

	// Check if value is empty (canceled)
	if newValue == "" {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("Empty value, not staged."))
		return nil
	}

	// Check if unchanged from staged value
	if stagedEntry != nil && newValue == currentValue {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No changes made."))
		return nil
	}

	// Stage the change with OperationCreate
	if err := r.Store.Stage(service, name, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     newValue,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Staged for creation: %s\n", green("✓"), name)
	return nil
}
