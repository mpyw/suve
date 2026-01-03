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

// EditRunner executes edit operations using a usecase.
type EditRunner struct {
	UseCase    *stagingusecase.EditUseCase
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
	// Get baseline value (staged value if exists, otherwise from AWS)
	baseline, err := r.UseCase.Baseline(ctx, stagingusecase.BaselineInput{Name: opts.Name})
	if err != nil {
		return err
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
		newValue, err = editorFn(baseline.Value)
		if err != nil {
			return fmt.Errorf("failed to edit: %w", err)
		}

		// Check if changed
		if newValue == baseline.Value {
			_, _ = fmt.Fprintln(r.Stdout, colors.Warning("No changes made."))
			return nil
		}
	}

	// Execute the edit use case
	result, err := r.UseCase.Execute(ctx, stagingusecase.EditInput{
		Name:        opts.Name,
		Value:       newValue,
		Description: opts.Description,
		Tags:        opts.Tags,
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Staged: %s\n", colors.Success("✓"), result.Name)
	return nil
}
