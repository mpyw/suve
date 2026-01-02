// Package diff provides the SSM Parameter Store diff command for comparing parameter versions.
package diff

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// Client is the interface for the diff command.
type Client interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
}

// Runner executes the diff command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	Spec1     *paramversion.Spec
	Spec2     *paramversion.Spec
	ParseJSON bool
	NoPager   bool
	Output    output.Format
}

// JSONOutput represents the JSON output structure for the diff command.
type JSONOutput struct {
	OldName    string `json:"oldName"`
	OldVersion int64  `json:"oldVersion"`
	OldValue   string `json:"oldValue"`
	NewName    string `json:"newName"`
	NewVersion int64  `json:"newVersion"`
	NewValue   string `json:"newValue"`
	Identical  bool   `json:"identical"`
	Diff       string `json:"diff,omitempty"`
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> <version1> [version2]",
		Description: `Compare two versions of a parameter in unified diff format.
If only one version/spec is specified, compares against latest.

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago; ~ alone means ~1

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
  suve param diff /app/config#1 /app/config#2   Compare v1 and v2 (full spec)
  suve param diff /app/config#3                 Compare v3 with latest (full spec)
  suve param diff /app/config#1 '#2'            Compare v1 and v2 (mixed)
  suve param diff /app/config '#1' '#2'         Compare v1 and v2 (partial spec)
  suve param diff /app/config '~'               Compare previous with latest
  suve param diff -j /app/config#1 /app/config  JSON format before diffing
  suve param diff --output=json /app/config#1   Output as JSON

For comparing staged values, use: suve stage param diff`,
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
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	spec1, spec2, err := paramversion.ParseDiffArgs(cmd.Args().Slice())
	if err != nil {
		return err
	}

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec1:     spec1,
		Spec2:     spec2,
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
		Output:    output.ParseFormat(cmd.String("output")),
	}

	// JSON output disables pager
	noPager := opts.NoPager || opts.Output == output.FormatJSON

	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		r := &Runner{
			Client: client,
			Stdout: w,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	param1, err := paramversion.GetParameterWithVersion(ctx, r.Client, opts.Spec1, true)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	param2, err := paramversion.GetParameterWithVersion(ctx, r.Client, opts.Spec2, true)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := lo.FromPtr(param1.Value)
	value2 := lo.FromPtr(param2.Value)

	// Format as JSON if enabled
	if opts.ParseJSON {
		value1, value2 = jsonutil.TryFormatOrWarn2(value1, value2, r.Stderr, "")
	}

	identical := value1 == value2

	// JSON output mode
	if opts.Output == output.FormatJSON {
		jsonOut := JSONOutput{
			OldName:    opts.Spec1.Name,
			OldVersion: param1.Version,
			OldValue:   value1,
			NewName:    opts.Spec2.Name,
			NewVersion: param2.Version,
			NewValue:   value2,
			Identical:  identical,
		}
		if !identical {
			jsonOut.Diff = output.DiffRaw(
				fmt.Sprintf("%s#%d", opts.Spec1.Name, param1.Version),
				fmt.Sprintf("%s#%d", opts.Spec2.Name, param2.Version),
				value1,
				value2,
			)
		}
		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonOut)
	}

	if identical {
		output.Warning(r.Stderr, "comparing identical versions")
		output.Hint(r.Stderr, "To compare with previous version, use: suve param diff %s~1", opts.Spec1.Name)
		return nil
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%d", opts.Spec1.Name, param1.Version),
		fmt.Sprintf("%s#%d", opts.Spec2.Name, param2.Version),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}
