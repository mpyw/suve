// Package log provides the generic log command shared by every provider.
//
// The scaffolding here owns everything identical across providers: flag parsing,
// the --since/--until RFC3339 parsing, the incompatible-option warnings, pager
// gating, the empty-history early return, and the per-mode dispatch loop. The
// 140-line multi-branch Run of the original twins is split into four modes
// (default, --patch, --oneline, --output=json), each delegating the per-version
// line formatting to the provider Presenter so output stays byte-identical.
package log

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
)

// Request holds the version-history fetch parameters (passed to the usecase).
type Request struct {
	Name       string
	MaxResults int32
	Since      *time.Time
	Until      *time.Time
	Reverse    bool
}

// Options holds the shared render options.
type Options struct {
	ShowPatch      bool
	ParseJSON      bool
	Reverse        bool
	NoPager        bool
	Oneline        bool
	Output         output.Format
	MaxValueLength int
}

// Presenter renders version history for a specific provider. Implementations are
// stateful: Fetch loads the history, and the render methods format each version.
type Presenter interface {
	// Fetch loads the version history via the provider usecase.
	Fetch(ctx context.Context) error
	// Len returns the number of versions loaded.
	Len() int
	// RenderJSON writes the provider-specific JSON array for all versions.
	RenderJSON(stdout io.Writer) error
	// RenderOneline writes the compact one-line form for version i.
	RenderOneline(stdout io.Writer, i, maxValueLength int)
	// RenderHeader writes the "Version ..." line and Date line for version i.
	RenderHeader(stdout io.Writer, i int)
	// RenderValue writes the value preview for version i (default, non-patch mode).
	// Providers without a default value preview (e.g. secrets) make this a no-op.
	RenderValue(stdout io.Writer, i, maxValueLength int)
	// RenderPatch writes the diff between version i and its neighbor (patch mode).
	RenderPatch(stdout, stderr io.Writer, i int, parseJSON, reverse bool)
}

// Runner executes the log command over a provider Presenter.
type Runner struct {
	Presenter Presenter
	Options   Options
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run executes the log command, dispatching to the appropriate per-mode
// rendering.
func (r *Runner) Run(ctx context.Context) error {
	if err := r.Presenter.Fetch(ctx); err != nil {
		return err
	}

	n := r.Presenter.Len()
	if n == 0 {
		return nil
	}

	// JSON output mode
	if r.Options.Output == output.FormatJSON {
		return r.Presenter.RenderJSON(r.Stdout)
	}

	for i := range n {
		if r.Options.Oneline && !r.Options.ShowPatch {
			r.Presenter.RenderOneline(r.Stdout, i, r.Options.MaxValueLength)

			continue
		}

		r.Presenter.RenderHeader(r.Stdout, i)

		if r.Options.ShowPatch {
			r.Presenter.RenderPatch(r.Stdout, r.Stderr, i, r.Options.ParseJSON, r.Options.Reverse)
		} else {
			r.Presenter.RenderValue(r.Stdout, i, r.Options.MaxValueLength)
		}

		if i < n-1 {
			output.Println(r.Stdout, "")
		}
	}

	return nil
}

// PatchParent computes, for version i in a fetched window of n versions, the
// index of its parent (the immediately-older version) and whether i is the
// oldest version in the window.
//
// git log -p attaches each version's own creation patch under its own header —
// the diff from its parent to itself. In newest-first order the parent is the
// next entry (i+1); in reverse (oldest-first) order it is the previous entry
// (i-1). The oldest version in the window has no parent in the list; the caller
// renders it as an all-added creation diff, but only when it is genuinely the
// initial version (guarding against a --number / date-filter window cut).
func PatchParent(i, n int, reverse bool) (parentIdx int, oldest bool) {
	if reverse {
		return i - 1, i == 0
	}

	return i + 1, i == n-1
}

// Config holds the provider-specific configuration for the log command.
type Config struct {
	// Usage is the one-line command usage string.
	Usage string
	// ArgsUsage is the positional-arguments usage string.
	ArgsUsage string
	// Description is the long help text.
	Description string
	// UsageError is returned when the resource name argument is missing.
	UsageError string
	// Flags is the provider's flag set (param adds --max-value-length; the
	// --number/--since/--until usage strings differ between providers).
	Flags []cli.Flag
	// NewPresenter builds the provider Presenter bound to the fetch request.
	NewPresenter func(ctx context.Context, req Request) (Presenter, error)
}

// Command returns the generic log command wired with the provider Config.
func Command(cfg Config) *cli.Command {
	return &cli.Command{
		Name:        "log",
		Aliases:     []string{"history"},
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Flags:       cfg.Flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 1 {
				return fmt.Errorf("%s", cfg.UsageError)
			}

			opts := Options{
				ShowPatch:      cmd.Bool("patch"),
				ParseJSON:      cmd.Bool("parse-json"),
				Reverse:        cmd.Bool("reverse"),
				NoPager:        cmd.Bool("no-pager"),
				Oneline:        cmd.Bool("oneline"),
				Output:         output.ParseFormat(cmd.String("output")),
				MaxValueLength: cmd.Int("max-value-length"),
			}

			req := Request{
				Name:       cmd.Args().First(),
				MaxResults: cmd.Int32("number"),
				Reverse:    opts.Reverse,
			}

			// Parse --since timestamp
			if sinceArg := cmd.String("since"); sinceArg != "" {
				since, err := time.Parse(time.RFC3339, sinceArg)
				if err != nil {
					return fmt.Errorf("invalid --since value: must be RFC3339 format (e.g., '2024-01-01T00:00:00Z')")
				}

				req.Since = &since
			}

			// Parse --until timestamp
			if untilArg := cmd.String("until"); untilArg != "" {
				until, err := time.Parse(time.RFC3339, untilArg)
				if err != nil {
					return fmt.Errorf("invalid --until value: must be RFC3339 format (e.g., '2024-12-31T23:59:59Z')")
				}

				req.Until = &until
			}

			// Warn if --parse-json is used without -p
			if opts.ParseJSON && !opts.ShowPatch {
				output.Warning(cmd.Root().ErrWriter, "--parse-json has no effect without -p/--patch")
			}

			// Warn if --oneline is used with -p
			if opts.Oneline && opts.ShowPatch {
				output.Warning(cmd.Root().ErrWriter, "--oneline has no effect with -p/--patch")
			}

			// Warn if --output=json is used with incompatible options
			if opts.Output == output.FormatJSON {
				if opts.ShowPatch {
					output.Warning(cmd.Root().ErrWriter, "-p/--patch has no effect with --output=json")
				}

				if opts.Oneline {
					output.Warning(cmd.Root().ErrWriter, "--oneline has no effect with --output=json")
				}
			}

			presenter, err := cfg.NewPresenter(ctx, req)
			if err != nil {
				return err
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
