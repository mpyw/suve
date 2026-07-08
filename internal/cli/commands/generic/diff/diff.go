// Package diff provides the generic diff command shared by every provider.
//
// The scaffolding here owns the flow that is identical across providers: diff
// argument parsing dispatch, parse-json formatting, the identical-versions
// check, pager gating, and the JSON/text dispatch. The provider-specific parts —
// the version-spec grammar, the diff header labels, the JSON shape, and the
// identical-versions hints — live behind the Presenter so each provider
// reproduces its own byte-identical output.
package diff

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
)

// Options holds the shared diff options.
type Options struct {
	ParseJSON bool
	NoPager   bool
	Output    output.Format
}

// Presenter renders a diff for a specific provider. Implementations are stateful:
// Fetch loads both versions, and the subsequent methods render them.
type Presenter interface {
	// Fetch resolves both specs and loads their entries via the provider usecase.
	Fetch(ctx context.Context) error
	// OldValue and NewValue return the two raw values to be compared.
	OldValue() string
	NewValue() string
	// Labels returns the unified-diff header labels for the old and new sides.
	Labels() (oldLabel, newLabel string)
	// RenderJSON writes the provider JSON output. diff is the pre-computed raw
	// unified diff (empty when the versions are identical).
	RenderJSON(stdout io.Writer, oldValue, newValue string, identical bool, diff string) error
	// Hints writes the provider-specific hints shown when versions are identical.
	Hints(stderr io.Writer)
}

// Runner executes the diff command over a provider Presenter.
type Runner struct {
	Presenter Presenter
	Options   Options
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context) error {
	if err := r.Presenter.Fetch(ctx); err != nil {
		return err
	}

	rawValue1 := r.Presenter.OldValue()
	rawValue2 := r.Presenter.NewValue()

	// The identical decision is made on the RAW stored values, so a --parse-json
	// reformat (whitespace, key order, number spelling) never masks a real
	// stored difference (previously the compare ran AFTER formatting).
	identical := rawValue1 == rawValue2

	// Format as JSON if enabled — for rendering only.
	value1, value2 := rawValue1, rawValue2
	if r.Options.ParseJSON {
		value1, value2 = jsonutil.TryFormatOrWarn2(value1, value2, r.Stderr, "")
	}

	oldLabel, newLabel := r.Presenter.Labels()

	// JSON output mode
	if r.Options.Output == output.FormatJSON {
		diff := ""
		if !identical {
			diff = output.DiffRaw(oldLabel, newLabel, value1, value2)
		}

		return r.Presenter.RenderJSON(r.Stdout, value1, value2, identical, diff)
	}

	if identical {
		// The values are byte-identical. Distinguish self-comparison from two
		// distinct versions that merely happen to hold the same content, and
		// only offer the self-comparison hints in the former case.
		if oldLabel == newLabel {
			output.Warning(r.Stderr, "comparing identical versions")
			r.Presenter.Hints(r.Stderr)
		} else {
			output.Warning(r.Stderr, "versions differ but content is identical")
		}

		return nil
	}

	diff := output.Diff(r.Stdout, oldLabel, newLabel, value1, value2)

	// The raw values differ but --parse-json normalized them to the same form:
	// there is no textual diff to show, so say so explicitly instead of printing
	// nothing (and never claim the versions are identical).
	if diff == "" {
		output.Warning(r.Stderr, "values differ only in JSON formatting")

		return nil
	}

	output.Print(r.Stdout, diff)

	return nil
}

// Config holds the provider-specific configuration for the diff command.
type Config[S any] struct {
	// Usage is the one-line command usage string.
	Usage string
	// ArgsUsage is the positional-arguments usage string.
	ArgsUsage string
	// Description is the long help text.
	Description string
	// ParseDiffArgs parses the raw positional args into two version specs.
	ParseDiffArgs func(args []string) (S, S, error)
	// NewPresenter builds the provider Presenter bound to the two specs.
	NewPresenter func(ctx context.Context, spec1, spec2 S) (Presenter, error)
}

// Command returns the generic diff command wired with the provider Config.
func Command[S any](cfg Config[S]) *cli.Command {
	return &cli.Command{
		Name:        "diff",
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			spec1, spec2, err := cfg.ParseDiffArgs(cmd.Args().Slice())
			if err != nil {
				return err
			}

			presenter, err := cfg.NewPresenter(ctx, spec1, spec2)
			if err != nil {
				return err
			}

			outputFormat, err := output.ParseFormat(cmd.String("output"))
			if err != nil {
				return err
			}

			opts := Options{
				ParseJSON: cmd.Bool("parse-json"),
				NoPager:   cmd.Bool("no-pager"),
				Output:    outputFormat,
			}

			// JSON output disables pager
			noPager := opts.NoPager || opts.Output == output.FormatJSON

			return internal.WithPager(cmd, noPager, func(stdout, stderr io.Writer) error {
				r := &Runner{Presenter: presenter, Options: opts, Stdout: stdout, Stderr: stderr}

				return r.Run(ctx)
			})
		},
	}
}
