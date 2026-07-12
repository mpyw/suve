package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/staging"
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
	// Namespace is the App Configuration namespace to stage under (empty for the
	// null/default namespace and every other provider).
	Namespace string
	// ValueType is the provider-neutral value type to record on the staged entry
	// (AWS SSM Parameter Store axis). Empty for providers without a type axis.
	ValueType domain.ValueType
}

// Run executes the add command.
func (r *AddRunner) Run(ctx context.Context, opts AddOptions) error {
	// Get draft (existing staged create value) for re-editing
	draft, err := r.UseCase.Draft(ctx, stagingusecase.DraftInput{Key: staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace}})
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

		newValue, err = editorFn(ctx, draft.Value)
		if err != nil {
			return fmt.Errorf("failed to edit: %w", err)
		}

		// Check if value is empty (canceled)
		if newValue == "" {
			output.Info(r.Stdout, "Empty value, not staged.")

			return nil
		}

		// Check if unchanged from staged value
		if draft.IsStaged && newValue == draft.Value {
			output.Info(r.Stdout, "No changes made.")

			return nil
		}
	}

	// Execute the add use case
	result, err := r.UseCase.Execute(ctx, stagingusecase.AddInput{
		Key:         staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace},
		Value:       newValue,
		Description: opts.Description,
		ValueType:   opts.ValueType,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Staged for creation: %s", result.Name)

	return nil
}
