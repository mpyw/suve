package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/editor"
)

// FlagValueStdin is the name of the flag that reads a create/update value from
// stdin instead of a positional argument, keeping the secret out of argv (and
// therefore out of ps/proc/cmdline and shell history).
const FlagValueStdin = "value-stdin"

// ValueStdinFlag returns the shared --value-stdin flag used by the direct
// create/update commands across every provider.
func ValueStdinFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  FlagValueStdin,
		Usage: "Read the value from stdin instead of a positional argument (keeps it out of argv/ps and shell history)",
	}
}

// Stdin returns the command's configured reader, falling back to os.Stdin when
// none is set. The production app leaves Reader unset, so this preserves the
// real stdin there while letting tests inject a reader through cmd.Root().Reader.
func Stdin(cmd *cli.Command) io.Reader {
	if r := cmd.Root().Reader; r != nil {
		return r
	}

	return os.Stdin
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
}

// ResolveValue determines the value for a create/update command. Precedence:
//
//  1. --value-stdin: read the whole of stdin (one trailing newline trimmed).
//  2. the positional value argument, when supplied.
//  3. $EDITOR fallback: open an empty buffer and use whatever is saved.
//
// proceed reports whether the command should continue. It is false only when
// the editor fallback returns an empty value, which is treated as a
// cancellation (matching the staging add/edit UX). --value-stdin and the
// positional argument always proceed, even with an empty value, because those
// are explicit.
func ResolveValue(ctx context.Context, src ValueSource) (value string, proceed bool, err error) {
	switch {
	case src.FromStdin:
		if src.HasArg {
			return "", false, errors.New("cannot combine a positional value with --" + FlagValueStdin)
		}

		reader := src.Stdin
		if reader == nil {
			reader = os.Stdin
		}

		data, rerr := io.ReadAll(reader)
		if rerr != nil {
			return "", false, fmt.Errorf("failed to read value from stdin: %w", rerr)
		}

		return trimTrailingNewline(string(data)), true, nil

	case src.HasArg:
		return src.Arg, true, nil

	default:
		openEditor := src.OpenEditor
		if openEditor == nil {
			openEditor = editor.Open
		}

		edited, eerr := openEditor(ctx, "")
		if eerr != nil {
			return "", false, fmt.Errorf("failed to edit value: %w", eerr)
		}

		return edited, edited != "", nil
	}
}

// trimTrailingNewline removes a single trailing newline (CRLF or LF), matching
// the trailing-newline handling of the external editor so that
// `printf '%s\n' secret | ... --value-stdin` and an editor session behave alike.
func trimTrailingNewline(s string) string {
	s = strings.TrimSuffix(s, "\r\n")
	s = strings.TrimSuffix(s, "\n")

	return s
}
