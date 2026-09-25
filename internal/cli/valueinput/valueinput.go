// Package valueinput resolves the value a write command takes: from stdin
// (--value-stdin), from a positional argument, or from $EDITOR when the session
// is interactive. The direct create/update commands and the stage add/edit
// commands share it, so a non-interactive session never waits on an editor.
package valueinput

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/cli/terminal"
)

// ErrValueRequired is returned when no value was provided and the editor
// fallback is unavailable because the session is non-interactive (a pipe, CI,
// or any non-TTY stdin). It never hangs waiting on an editor.
var ErrValueRequired = errors.New(
	"value is required: pass it as an argument, pipe it in with --" + FlagValueStdin +
		", or run interactively to edit it in $EDITOR",
)

// ErrValueStdinNeedsYes is returned when the value was read from stdin via
// --value-stdin but a confirmation prompt would still run afterwards. The
// prompt reads from the same stdin, which has already been consumed, so it
// would immediately hit EOF. Rather than fail cryptically, we detect this up
// front and tell the user to re-run with --yes. We intentionally neither imply
// --yes nor read the confirmation from /dev/tty, so the acknowledgement stays
// explicit.
var ErrValueStdinNeedsYes = errors.New(
	"--" + FlagValueStdin + " consumes stdin, so the confirmation prompt cannot be read; " +
		"re-run with --yes to acknowledge the update",
)

// FlagValueStdin is the name of the flag that reads a create/update value from
// stdin instead of a positional argument, keeping the secret out of argv (and
// therefore out of ps/proc/cmdline and shell history).
const FlagValueStdin = "value-stdin"

// ValueStdinFlag returns the shared --value-stdin flag used by the direct
// create/update and stage add/edit commands across every provider.
func ValueStdinFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  FlagValueStdin,
		Usage: "Read the value from stdin instead of a positional argument (keeps it out of argv/ps and shell history)",
	}
}

// ValueStdin returns the command's configured reader, falling back to os.Stdin when
// none is set. The production app leaves Reader unset, so this preserves the
// real stdin there while letting tests inject a reader through cmd.Root().Reader.
func ValueStdin(cmd *cli.Command) io.Reader {
	if r := cmd.Root().Reader; r != nil {
		return r
	}

	return os.Stdin
}

// interactiveValueReader reports whether r is a terminal, i.e. whether it is safe to
// fall back to $EDITOR. os.Stdin is a terminal in an interactive shell but not
// under a pipe or CI, so this prevents the editor fallback from hanging in a
// non-interactive session. A nil reader (unset) is treated as non-interactive.
func interactiveValueReader(r io.Reader) bool {
	f, ok := r.(terminal.Fder)

	return ok && terminal.IsTTY(f.Fd())
}

// ValueSource describes where a create/update value may come from. Exactly one
// of the non-editor sources is selected by ResolveValue's precedence rules.
type ValueSource struct {
	// FromStdin is true when --value-stdin was given.
	FromStdin bool
	// HasArg is true when a positional value argument was supplied.
	HasArg bool
	// Arg is the positional value argument (only meaningful when HasArg).
	Arg string
	// Stdin is the reader used when FromStdin is true (nil -> os.Stdin).
	Stdin io.Reader
	// OpenEditor is the editor seam used for the fallback path (nil -> editor.Open).
	OpenEditor editor.OpenFunc
	// EditorInitial is the text the editor fallback opens with (e.g. the staged
	// draft or the current value); empty opens a blank buffer.
	EditorInitial string
	// ConfirmRequired is true when the command would prompt for confirmation on
	// the same stdin after resolving the value (i.e. an update without --yes).
	// Combined with FromStdin this is the double-consume case, so ResolveValue
	// fails with ErrValueStdinNeedsYes instead of letting the later prompt hit
	// EOF.
	ConfirmRequired bool
}

// ResolveValue determines the value for a create/update or stage add/edit command. Precedence:
//
//  1. --value-stdin: read the whole of stdin (one trailing newline trimmed).
//  2. the positional value argument, when supplied.
//  3. $EDITOR fallback: open EditorInitial (usually empty) and use whatever is
//     saved. A non-interactive stdin fails with ErrValueRequired instead.
//
// proceed reports whether the command should continue. It is false only when
// the editor fallback returns an empty value, which is treated as a
// cancellation (matching the staging add/edit UX). --value-stdin and the
// positional argument always proceed, even with an empty value, because those
// are explicit.
//
// When ConfirmRequired is set alongside --value-stdin, ResolveValue returns
// ErrValueStdinNeedsYes instead of reading stdin, because the later
// confirmation prompt would find stdin already consumed.
func ResolveValue(ctx context.Context, src ValueSource) (value string, proceed bool, err error) {
	switch {
	case src.FromStdin:
		if src.HasArg {
			return "", false, errors.New("cannot combine a positional value with --" + FlagValueStdin)
		}

		if src.ConfirmRequired {
			return "", false, ErrValueStdinNeedsYes
		}

		reader := src.Stdin
		if reader == nil {
			reader = os.Stdin
		}

		data, rerr := io.ReadAll(reader)
		if rerr != nil {
			return "", false, fmt.Errorf("failed to read value from stdin: %w", rerr)
		}

		return trimValueTrailingNewline(string(data)), true, nil

	case src.HasArg:
		return src.Arg, true, nil

	default:
		if err := CheckEditorFallback(src); err != nil {
			return "", false, err
		}

		openEditor := src.OpenEditor
		if openEditor == nil {
			openEditor = editor.Open
		}

		edited, eerr := openEditor(ctx, src.EditorInitial)
		if eerr != nil {
			return "", false, fmt.Errorf("failed to edit value: %w", eerr)
		}

		return edited, edited != "", nil
	}
}

// CheckEditorFallback returns ErrValueRequired when src names no value (no
// --value-stdin, no argument) and the $EDITOR fallback cannot run. The real
// $EDITOR is a blocking, interactive program: launching it under a pipe or in
// CI would hang forever, so it runs only when stdin is a TTY. An injected
// OpenEditor (tests) is exempt. Callers that must do slow work before
// ResolveValue (e.g. fetch the value to edit) call it first to fail early.
func CheckEditorFallback(src ValueSource) error {
	if src.FromStdin || src.HasArg || src.OpenEditor != nil {
		return nil
	}

	if !interactiveValueReader(src.Stdin) {
		return ErrValueRequired
	}

	return nil
}

// trimValueTrailingNewline removes a single trailing newline (CRLF or LF), matching
// the trailing-newline handling of the external editor so that
// `printf '%s\n' secret | ... --value-stdin` and an editor session behave alike.
func trimValueTrailingNewline(s string) string {
	s = strings.TrimSuffix(s, "\r\n")
	s = strings.TrimSuffix(s, "\n")

	return s
}
