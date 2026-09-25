package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// AddRunner executes add operations using a usecase.
type AddRunner struct {
	UseCase *stagingusecase.AddUseCase
	Stdout  io.Writer
	Stderr  io.Writer
	// Stdin is read for --value-stdin and decides whether the $EDITOR fallback
	// may run (only on a terminal). Nil is treated as a non-interactive stdin.
	Stdin      io.Reader
	OpenEditor editor.OpenFunc // Optional: defaults to editor.Open (TTY only) if nil
}

// AddOptions holds options for the add command.
type AddOptions struct {
	Name string
	// Value is the explicit value; it is used only when HasValue is set.
	Value string
	// HasValue reports that a value argument was given, even an empty one.
	HasValue bool
	// ValueFromStdin reads the value from Stdin (--value-stdin).
	ValueFromStdin bool
	Description    string
	// Namespace is the App Configuration namespace to stage under (empty for the
	// null/default namespace and every other provider).
	Namespace string
	// ValueType is the provider-neutral value type to record on the staged entry
	// (AWS Parameter Store axis). Empty for providers without a type axis.
	ValueType domain.ValueType
}

// Run executes the add command.
func (r *AddRunner) Run(ctx context.Context, opts AddOptions) error {
	// Get draft (existing staged create value) for re-editing
	draft, err := r.UseCase.Draft(ctx, stagingusecase.DraftInput{Key: staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace}})
	if err != nil {
		return err
	}

	newValue, proceed, err := valueinput.ResolveValue(ctx, valueinput.ValueSource{
		FromStdin:     opts.ValueFromStdin,
		HasArg:        opts.HasValue,
		Arg:           opts.Value,
		Stdin:         r.Stdin,
		OpenEditor:    r.OpenEditor,
		EditorInitial: draft.Value,
	})
	if err != nil {
		return err
	}

	// An explicit value (argument or stdin) is staged as given. Only the
	// editor result is checked for a cancel or a no-op.
	if !opts.ValueFromStdin && !opts.HasValue {
		// Check if value is empty (canceled)
		if !proceed {
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
