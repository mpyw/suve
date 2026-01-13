// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/cli/output"
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
		newValue, err = editorFn(ctx, baseline.Value)
		if err != nil {
			return fmt.Errorf("failed to edit: %w", err)
		}

		// Check if changed
		if newValue == baseline.Value {
			output.Info(r.Stdout, "No changes made.")
			return nil
		}
	}

	// Execute the edit use case
	result, err := r.UseCase.Execute(ctx, stagingusecase.EditInput{
		Name:        opts.Name,
		Value:       newValue,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	switch {
	case result.Skipped:
		output.Warn(r.Stdout, "Skipped %s (same as AWS)", result.Name)
	case result.Unstaged:
		output.Success(r.Stdout, "Unstaged %s (reverted to AWS)", result.Name)
	default:
		output.Success(r.Stdout, "Staged: %s", result.Name)
	}
	return nil
}
