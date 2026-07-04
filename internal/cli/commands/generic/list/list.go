// Package list provides the generic list command shared by every provider.
//
// The output rendering (names-only, --show text, and JSON forms) is identical
// across providers; only the small per-provider Config (help text, flag set,
// and the provider usecase wiring that produces the entries) varies. Note that
// list does not use a pager: it writes straight to the root writer.
package list

import (
	"context"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
)

// Entry is a provider-neutral list row: a name plus an optional value or error.
type Entry struct {
	Name  string
	Value *string
	Error error
}

// Options holds the shared list options.
type Options struct {
	Show   bool
	Output output.Format
}

// JSONOutputItem represents a single item in JSON output.
type JSONOutputItem struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"` // nil when error or not requested, pointer to distinguish from empty string
	Error string  `json:"error,omitempty"`
}

// Config holds the provider-specific configuration for the list command.
type Config struct {
	// Usage is the one-line command usage string.
	Usage string
	// ArgsUsage is the positional-arguments usage string.
	ArgsUsage string
	// Description is the long help text.
	Description string
	// Flags is the provider's flag set (param adds --recursive; secret does not).
	Flags []cli.Flag
	// NewList builds the entry-producing closure from the CLI context. withValue
	// mirrors the shared --show flag so the provider can request values.
	NewList func(ctx context.Context, cmd *cli.Command, withValue bool) (func(context.Context) ([]Entry, error), error)
}

// Runner executes the list command over a provider-supplied entry source.
type Runner struct {
	List    func(ctx context.Context) ([]Entry, error)
	Options Options
	Stdout  io.Writer
	Stderr  io.Writer
}

// Run executes the list command.
func (r *Runner) Run(ctx context.Context) error {
	entries, err := r.List(ctx)
	if err != nil {
		return err
	}

	// If --show is not specified and not JSON output, just print names
	if !r.Options.Show && r.Options.Output != output.FormatJSON {
		for _, entry := range entries {
			output.Println(r.Stdout, entry.Name)
		}

		return nil
	}

	// For JSON output without --show, output names only
	if r.Options.Output == output.FormatJSON && !r.Options.Show {
		items := make([]JSONOutputItem, len(entries))
		for i, entry := range entries {
			items[i] = JSONOutputItem{Name: entry.Name}
		}

		return output.WriteJSON(r.Stdout, items)
	}

	// JSON output with values
	if r.Options.Output == output.FormatJSON {
		items := make([]JSONOutputItem, 0, len(entries))
		for _, entry := range entries {
			if entry.Error != nil {
				items = append(items, JSONOutputItem{Name: entry.Name, Error: entry.Error.Error()})
			} else {
				items = append(items, JSONOutputItem{Name: entry.Name, Value: entry.Value})
			}
		}

		return output.WriteJSON(r.Stdout, items)
	}

	// Text output with values
	for _, entry := range entries {
		if entry.Error != nil {
			output.Printf(r.Stdout, "%s\t<error: %v>\n", entry.Name, entry.Error)
		} else {
			output.Printf(r.Stdout, "%s\t%s\n", entry.Name, lo.FromPtr(entry.Value))
		}
	}

	return nil
}

// Command returns the generic list command wired with the provider Config.
func Command(cfg Config) *cli.Command {
	return &cli.Command{
		Name:        "list",
		Aliases:     []string{"ls"},
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Flags:       cfg.Flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			opts := Options{
				Show:   cmd.Bool("show"),
				Output: output.ParseFormat(cmd.String("output")),
			}

			list, err := cfg.NewList(ctx, cmd, opts.Show)
			if err != nil {
				return err
			}

			r := &Runner{
				List:    list,
				Options: opts,
				Stdout:  cmd.Root().Writer,
				Stderr:  cmd.Root().ErrWriter,
			}

			return r.Run(ctx)
		},
	}
}
