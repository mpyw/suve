// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/editor"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// AddRunner executes add operations using a usecase.
type AddRunner struct {
	UseCase    *stagingusecase.AddUseCase
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
func (r *AddRunner) Run(ctx context.Context, opts AddOptions) error {
	// Get draft (existing staged create value) for re-editing
	draft, err := r.UseCase.Draft(ctx, stagingusecase.DraftInput{Name: opts.Name})
	if err != nil {
		return err
	}

	var newValue string
	if opts.Value != "" {
		// Use provided value, skip editor
		newValue = opts.Value
	} else {
		// Open editor with current draft value
		editorFn := r.OpenEditor
		if editorFn == nil {
			editorFn = editor.Open
		}
		newValue, err = editorFn(draft.Value)
		if err != nil {
			return fmt.Errorf("failed to edit: %w", err)
		}

		// Check if value is empty (canceled)
		if newValue == "" {
			_, _ = fmt.Fprintln(r.Stdout, colors.Warning("Empty value, not staged."))
			return nil
		}

		// Check if unchanged from staged value
		if draft.IsStaged && newValue == draft.Value {
			_, _ = fmt.Fprintln(r.Stdout, colors.Warning("No changes made."))
			return nil
		}
	}

	// Execute the add use case
	result, err := r.UseCase.Execute(ctx, stagingusecase.AddInput{
		Name:        opts.Name,
		Value:       newValue,
		Description: opts.Description,
		Tags:        opts.Tags,
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Staged for creation: %s\n", colors.Success("✓"), result.Name)
	return nil
}
