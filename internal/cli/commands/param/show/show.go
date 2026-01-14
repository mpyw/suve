// Package show provides the SSM Parameter Store show command.
package show

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// Runner executes the show command.
type Runner struct {
	UseCase *param.ShowUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the show command.
type Options struct {
	Spec      *paramversion.Spec
	ParseJSON bool
	NoPager   bool
	Raw       bool
	Output    output.Format
}

// JSONOutput represents the JSON output structure for the show command.
type JSONOutput struct {
	Name       string            `json:"name"`
	Version    int64             `json:"version"`
	Type       string            `json:"type"`
	JSONParsed *bool             `json:"json_parsed,omitempty"` //nolint:tagliatelle // snake_case for backwards compatibility
	Modified   string            `json:"modified,omitempty"`
	Tags       map[string]string `json:"tags"`
	Value      string            `json:"value"`
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show parameter value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a parameter's value along with its metadata (name, version, type, modification date).

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago (e.g., ~1, ~2); ~ alone means ~1

EXAMPLES:
  suve param show /app/config                               Show latest version
  suve param show /app/config~                              Show previous version
  suve param show /app/config#3                             Show version 3
  suve param show --raw /app/config                         Output raw value (for piping)
  suve param show --parse-json /app/config                  Pretty print JSON value
  suve param show --output=json /app/config                 Output as JSON
  DB_URL=$(suve param show --raw /app/config)               Use in shell variable`,
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
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve param show <name>")
	}

	spec, err := paramversion.Parse(cmd.Args().First())
	if err != nil {
		return err
	}

	outputFormat := output.ParseFormat(cmd.String("output"))
	raw := cmd.Bool("raw")

	// Check mutually exclusive options
	if raw && outputFormat == output.FormatJSON {
		return fmt.Errorf("--raw and --output=json cannot be used together")
	}

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec:      spec,
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
		Raw:       raw,
		Output:    outputFormat,
	}

	// Raw mode and JSON output disable pager
	noPager := opts.NoPager || opts.Raw || opts.Output == output.FormatJSON

	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		r := &Runner{
			UseCase: &param.ShowUseCase{Client: client},
			Stdout:  w,
			Stderr:  cmd.Root().ErrWriter,
		}

		return r.Run(ctx, opts)
	})
}

// Run executes the show command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.ShowInput{
		Spec: opts.Spec,
	})
	if err != nil {
		return err
	}

	value := result.Value
	jsonParsed := false

	// Warn if --parse-json is used in cases where it's not meaningful
	if opts.ParseJSON {
		switch result.Type {
		case paramapi.ParameterTypeStringList:
			output.Warning(r.Stderr, "--parse-json has no effect on StringList type (comma-separated values)")
		default:
			formatted := jsonutil.TryFormatOrWarn(value, r.Stderr, "")
			if formatted != value {
				jsonParsed = true
				value = formatted
			}
		}
	}

	// Raw mode: output value only without trailing newline
	if opts.Raw {
		output.Print(r.Stdout, value)

		return nil
	}

	// JSON output mode
	if opts.Output == output.FormatJSON {
		jsonOut := JSONOutput{
			Name:    result.Name,
			Version: result.Version,
			Type:    string(result.Type),
			Value:   value,
		}
		// Show json_parsed only when --parse-json was used and succeeded
		if jsonParsed {
			jsonOut.JSONParsed = lo.ToPtr(true)
		}

		if result.LastModified != nil {
			jsonOut.Modified = timeutil.FormatRFC3339(*result.LastModified)
		}

		jsonOut.Tags = make(map[string]string)
		for _, tag := range result.Tags {
			jsonOut.Tags[tag.Key] = tag.Value
		}

		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(jsonOut)
	}

	// Normal mode: show metadata + value
	out := output.New(r.Stdout)
	out.Field("Name", result.Name)
	out.Field("Version", strconv.FormatInt(result.Version, 10))
	out.Field("Type", string(result.Type))
	// Show json_parsed only when --parse-json was used and succeeded
	if jsonParsed {
		out.Field("JsonParsed", "true")
	}

	if result.LastModified != nil {
		out.Field("Modified", timeutil.FormatRFC3339(*result.LastModified))
	}

	if len(result.Tags) > 0 {
		out.Field("Tags", fmt.Sprintf("%d tag(s)", len(result.Tags)))

		for _, tag := range result.Tags {
			out.Field("  "+tag.Key, tag.Value)
		}
	}

	out.Separator()
	out.Value(value)

	return nil
}
