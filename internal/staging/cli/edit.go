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

// editRunner executes edit operations using a usecase.
//
//declscope:package // command.go builds and runs it
type editRunner struct {
	useCase *stagingusecase.EditUseCase
	// providerLabel names the remote store in messages (e.g. "AWS"); empty
	// renders as "remote".
	providerLabel string
	stdout        io.Writer
	stderr        io.Writer
	// stdin is read for --value-stdin and decides whether the $EDITOR fallback
	// may run (only on a terminal). Nil is treated as a non-interactive stdin.
	stdin      io.Reader
	openEditor editor.OpenFunc // Optional: defaults to editor.Open (TTY only) if nil
}

// editOptions holds options for the edit command.
//
//declscope:package // command.go fills it from the flags
type editOptions struct {
	name string
	// value is the explicit value; it is used only when hasValue is set.
	value string
	// hasValue reports that a value argument was given, even an empty one.
	hasValue bool
	// valueFromStdin reads the value from Stdin (--value-stdin).
	valueFromStdin bool
	description    string
	// namespace is the App Configuration namespace of the setting (empty for the
	// null/default namespace and every other provider).
	namespace string
	// valueType is the provider-neutral value type to record on the staged entry
	// (AWS Parameter Store axis). Empty preserves the existing type.
	valueType domain.ValueType
}

// run executes the edit command.
//
//declscope:package // command.go runs it
func (r *editRunner) run(ctx context.Context, opts editOptions) error {
	src := valueinput.ValueSource{
		FromStdin:  opts.valueFromStdin,
		HasArg:     opts.hasValue,
		Arg:        opts.value,
		Stdin:      r.stdin,
		OpenEditor: r.openEditor,
	}

	// Fail before fetching the remote baseline when there is no value and no
	// editor can run.
	if err := valueinput.CheckEditorFallback(src); err != nil {
		return err
	}

	// Get baseline value (staged value if exists, otherwise from the remote store)
	baseline, err := r.useCase.Baseline(ctx, stagingusecase.BaselineInput{Key: staging.EntryKey{Name: opts.name, Namespace: opts.namespace}})
	if err != nil {
		return err
	}

	// The editor's proceed flag is ignored: clearing the value in the editor
	// stages an empty value, as before.
	src.EditorInitial = baseline.Value

	newValue, _, err := valueinput.ResolveValue(ctx, src)
	if err != nil {
		return err
	}

	// An explicit value (argument or stdin) goes to the use case as given; it
	// reports a value equal to the remote one as skipped. Only an unchanged
	// editor result is a no-op here.
	if !opts.valueFromStdin && !opts.hasValue && newValue == baseline.Value {
		output.Info(r.stdout, "No changes made.")

		return nil
	}

	// Execute the edit use case
	result, err := r.useCase.Execute(ctx, stagingusecase.EditInput{
		Key:         staging.EntryKey{Name: opts.name, Namespace: opts.namespace},
		Value:       newValue,
		Description: opts.description,
		ValueType:   opts.valueType,
	})
	if err != nil {
		return err
	}

	switch {
	case result.Skipped:
		output.Warn(r.stdout, "Skipped %s (same as %s)", result.Name, remoteName(r.providerLabel))
	case result.Unstaged:
		output.Success(r.stdout, "Unstaged %s (reverted to %s)", result.Name, remoteName(r.providerLabel))
	default:
		output.Success(r.stdout, "Staged: %s", result.Name)
	}

	return nil
}
