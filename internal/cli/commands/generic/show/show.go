// Package show provides the generic show command shared by every provider.
//
// The scaffolding here owns the control flow that is identical across providers:
// argument validation, the --raw/--output=json mutual-exclusion check, pager
// gating, and the raw/json/text dispatch. Everything that differs between
// providers — the version-spec grammar, the parse-json rules, and the exact
// metadata field layout / JSON shape — lives behind the Presenter, so each
// provider reproduces its own byte-identical output.
package show

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
)

// Options holds the shared show options.
type Options struct {
	ParseJSON bool
	NoPager   bool
	Raw       bool
	Output    output.Format
}

// Presenter renders a single show result for a specific provider. Implementations
// are stateful: Fetch loads the entry, and the subsequent methods render it.
type Presenter interface {
	// Fetch resolves the version spec and loads the entry via the provider usecase.
	Fetch(ctx context.Context) error
	// Value returns the display value. When parseJSON is set it applies the
	// provider's parse-json rules (which may warn to stderr) and records internal
	// state used by the render methods.
	Value(parseJSON bool, stderr io.Writer) string
	// RenderText writes the metadata block followed by the value.
	RenderText(stdout io.Writer, value string)
	// RenderJSON writes the provider-specific JSON output.
	RenderJSON(stdout io.Writer, value string) error
}

// Runner executes the show command over a provider Presenter.
type Runner struct {
	Presenter Presenter
	Options   Options
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run executes the show command.
func (r *Runner) Run(ctx context.Context) error {
	if err := r.Presenter.Fetch(ctx); err != nil {
		return err
	}

	value := r.Presenter.Value(r.Options.ParseJSON, r.Stderr)

	// Raw mode: output value only without trailing newline
	if r.Options.Raw {
		output.Print(r.Stdout, value)

		return nil
	}

	// JSON output mode
	if r.Options.Output == output.FormatJSON {
		return r.Presenter.RenderJSON(r.Stdout, value)
	}

	// Normal mode: show metadata + value
	r.Presenter.RenderText(r.Stdout, value)

	return nil
}

// Config holds the provider-specific configuration for the show command.
type Config[S any] struct {
	// Usage is the one-line command usage string.
	Usage string
	// ArgsUsage is the positional-arguments usage string.
	ArgsUsage string
	// Description is the long help text.
	Description string
	// UsageError is returned when the resource name argument is missing.
	UsageError string
	// ParseSpec parses the raw name argument into the provider's version spec.
	ParseSpec func(arg string) (S, error)
	// NewPresenter builds the provider Presenter bound to the parsed spec (this
	// is where the provider constructs its AWS client and usecase).
	NewPresenter func(ctx context.Context, spec S) (Presenter, error)
}

// Command returns the generic show command wired with the provider Config.
func Command[S any](cfg Config[S]) *cli.Command {
	return &cli.Command{
		Name:        "show",
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values (keys are always sorted alphabetically)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.BoolFlag{
				Name:  "raw",
				Usage: "Output raw value only without metadata (for piping)",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("%s", cfg.UsageError)
			}

			spec, err := cfg.ParseSpec(cmd.Args().First())
			if err != nil {
				return err
			}

			outputFormat := output.ParseFormat(cmd.String("output"))
			raw := cmd.Bool("raw")

			// Check mutually exclusive options
			if raw && outputFormat == output.FormatJSON {
				return fmt.Errorf("--raw and --output=json cannot be used together")
			}

			presenter, err := cfg.NewPresenter(ctx, spec)
			if err != nil {
				return err
			}

			opts := Options{
				ParseJSON: cmd.Bool("parse-json"),
				NoPager:   cmd.Bool("no-pager"),
				Raw:       raw,
				Output:    outputFormat,
			}

			// Raw mode and JSON output disable pager
			noPager := opts.NoPager || opts.Raw || opts.Output == output.FormatJSON

			return internal.WithPager(cmd, noPager, func(stdout, stderr io.Writer) error {
				r := &Runner{Presenter: presenter, Options: opts, Stdout: stdout, Stderr: stderr}

				return r.Run(ctx)
			})
		},
	}
}
