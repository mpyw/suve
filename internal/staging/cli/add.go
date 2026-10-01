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

// addRunner executes add operations using a usecase.
//
//declscope:package // command.go builds and runs it
type addRunner struct {
	useCase *stagingusecase.AddUseCase
	stdout  io.Writer
	stderr  io.Writer
	// stdin is read for --value-stdin and decides whether the $EDITOR fallback
	// may run (only on a terminal). Nil is treated as a non-interactive stdin.
	stdin io.Reader
	//declscope:private // only add.go opens the editor
	openEditor editor.OpenFunc // Optional: defaults to editor.Open (TTY only) if nil
}

// addOptions holds options for the add command.
//
//declscope:package // command.go fills it from the flags
type addOptions struct {
	name string
	// value is the explicit value; it is used only when hasValue is set.
	value string
	// hasValue reports that a value argument was given, even an empty one.
	hasValue bool
	// valueFromStdin reads the value from Stdin (--value-stdin).
	valueFromStdin bool
	description    string
	// namespace is the App Configuration namespace to stage under (empty for the
	// null/default namespace and every other provider).
	namespace string
	// valueType is the provider-neutral value type to record on the staged entry
	// (AWS Parameter Store axis). Empty for providers without a type axis.
	valueType domain.ValueType
}

// run executes the add command.
//
//declscope:package // command.go runs it
func (r *addRunner) run(ctx context.Context, opts addOptions) error {
	// Get draft (existing staged create value) for re-editing
	draft, err := r.useCase.Draft(ctx, stagingusecase.DraftInput{Key: staging.EntryKey{Name: opts.name, Namespace: opts.namespace}})
	if err != nil {
		return err
	}

	newValue, proceed, err := valueinput.ResolveValue(ctx, valueinput.ValueSource{
		FromStdin:     opts.valueFromStdin,
		HasArg:        opts.hasValue,
		Arg:           opts.value,
		Stdin:         r.stdin,
		OpenEditor:    r.openEditor,
		EditorInitial: draft.Value,
	})
	if err != nil {
		return err
	}

	// An explicit value (argument or stdin) is staged as given. Only the
	// editor result is checked for a cancel or a no-op.
	if !opts.valueFromStdin && !opts.hasValue {
		// Check if value is empty (canceled)
		if !proceed {
			output.Info(r.stdout, "Empty value, not staged.")

			return nil
		}

		// Check if unchanged from staged value
		if draft.IsStaged && newValue == draft.Value {
			output.Info(r.stdout, "No changes made.")

			return nil
		}
	}

	// Execute the add use case
	result, err := r.useCase.Execute(ctx, stagingusecase.AddInput{
		Key:         staging.EntryKey{Name: opts.name, Namespace: opts.namespace},
		Value:       newValue,
		Description: opts.description,
		ValueType:   opts.valueType,
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Staged for creation: %s", result.Name)

	return nil
}
