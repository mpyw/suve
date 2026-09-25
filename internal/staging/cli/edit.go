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

// EditRunner executes edit operations using a usecase.
type EditRunner struct {
	UseCase *stagingusecase.EditUseCase
	// ProviderLabel names the remote store in messages (e.g. "AWS"); empty
	// renders as "remote".
	ProviderLabel string
	Stdout        io.Writer
	Stderr        io.Writer
	// Stdin is read for --value-stdin and decides whether the $EDITOR fallback
	// may run (only on a terminal). Nil is treated as a non-interactive stdin.
	Stdin      io.Reader
	OpenEditor editor.OpenFunc // Optional: defaults to editor.Open (TTY only) if nil
}

// EditOptions holds options for the edit command.
type EditOptions struct {
	Name string
	// Value is the explicit value; it is used only when HasValue is set.
	Value string
	// HasValue reports that a value argument was given, even an empty one.
	HasValue bool
	// ValueFromStdin reads the value from Stdin (--value-stdin).
	ValueFromStdin bool
	Description    string
	// Namespace is the App Configuration namespace of the setting (empty for the
	// null/default namespace and every other provider).
	Namespace string
	// ValueType is the provider-neutral value type to record on the staged entry
	// (AWS Parameter Store axis). Empty preserves the existing type.
	ValueType domain.ValueType
}

// Run executes the edit command.
func (r *EditRunner) Run(ctx context.Context, opts EditOptions) error {
	src := valueinput.ValueSource{
		FromStdin:  opts.ValueFromStdin,
		HasArg:     opts.HasValue,
		Arg:        opts.Value,
		Stdin:      r.Stdin,
		OpenEditor: r.OpenEditor,
	}

	// Fail before fetching the remote baseline when there is no value and no
	// editor can run.
	if err := valueinput.CheckEditorFallback(src); err != nil {
		return err
	}

	// Get baseline value (staged value if exists, otherwise from the remote store)
	baseline, err := r.UseCase.Baseline(ctx, stagingusecase.BaselineInput{Key: staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace}})
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
	if !opts.ValueFromStdin && !opts.HasValue && newValue == baseline.Value {
		output.Info(r.Stdout, "No changes made.")

		return nil
	}

	// Execute the edit use case
	result, err := r.UseCase.Execute(ctx, stagingusecase.EditInput{
		Key:         staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace},
		Value:       newValue,
		Description: opts.Description,
		ValueType:   opts.ValueType,
	})
	if err != nil {
		return err
	}

	switch {
	case result.Skipped:
		output.Warn(r.Stdout, "Skipped %s (same as %s)", result.Name, remoteName(r.ProviderLabel))
	case result.Unstaged:
		output.Success(r.Stdout, "Unstaged %s (reverted to %s)", result.Name, remoteName(r.ProviderLabel))
	default:
		output.Success(r.Stdout, "Staged: %s", result.Name)
	}

	return nil
}
