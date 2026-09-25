package param

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version"
)

// showJSONOutput represents the JSON output structure for the show command.
type showJSONOutput struct {
	Name        string            `json:"name"`
	Version     int64             `json:"version"`
	Type        string            `json:"type"`
	JSONParsed  *bool             `json:"json_parsed,omitempty"` //nolint:tagliatelle // snake_case for backwards compatibility
	Modified    string            `json:"modified,omitempty"`
	Description string            `json:"description,omitempty"`
	Tags        map[string]string `json:"tags"`
	Value       string            `json:"value"`
}

// showPresenter renders SSM Parameter Store show output byte-for-byte as before.
type showPresenter struct {
	uc         *param.ShowUseCase
	spec       *version.NumericSpec
	result     *param.ShowOutput
	jsonParsed bool
}

// NewShowPresenter builds a param show presenter over the given reader and spec.
// It is exported for the shared golden-output test harness.
func NewShowPresenter(reader provider.Reader, spec *version.NumericSpec) generic.ShowPresenter {
	return &showPresenter{uc: &param.ShowUseCase{Reader: reader}, spec: spec}
}

func (p *showPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, param.ShowInput{Name: p.spec.Name, Suffix: version.AWSParameterStore.Suffix(p.spec)})
	if err != nil {
		return err
	}

	p.result = result

	return nil
}

func (p *showPresenter) Value(parseJSON bool, stderr io.Writer) string {
	value := p.result.Value

	// Warn if --parse-json is used in cases where it's not meaningful
	if parseJSON {
		switch p.result.Type {
		case domain.ValueTypeList:
			output.Warning(stderr, "--parse-json has no effect on StringList type (comma-separated values)")
		default:
			formatted := jsonutil.TryFormatOrWarn(value, stderr, "")
			if formatted != value {
				p.jsonParsed = true
				value = formatted
			}
		}
	}

	return value
}

func (p *showPresenter) RenderText(stdout io.Writer, value string) {
	result := p.result

	out := output.New(stdout)
	out.Field("Name", result.Name)
	out.Field("Version", result.Version)
	out.Field("Type", paramtype.Display(result.Type))
	// Show json_parsed only when --parse-json was used and succeeded
	if p.jsonParsed {
		out.Field("JsonParsed", "true")
	}

	if result.LastModified != nil {
		out.Field("Modified", timeutil.FormatRFC3339(*result.LastModified))
	}

	if result.Description != "" {
		out.Field("Description", result.Description)
	}

	if len(result.Tags) > 0 {
		out.Field("Tags", fmt.Sprintf("%d tag(s)", len(result.Tags)))

		for _, tag := range result.Tags {
			out.Field("  "+tag.Key, tag.Value)
		}
	}

	out.Separator()
	out.Value(value)
}

func (p *showPresenter) RenderJSON(stdout io.Writer, value string) error {
	result := p.result

	jsonOut := showJSONOutput{
		Name:    result.Name,
		Version: versionNumber(result.Version),
		Type:    paramtype.Display(result.Type),
		Value:   value,
	}
	// Show json_parsed only when --parse-json was used and succeeded
	if p.jsonParsed {
		jsonOut.JSONParsed = new(true)
	}

	if result.LastModified != nil {
		jsonOut.Modified = timeutil.FormatRFC3339(*result.LastModified)
	}

	if result.Description != "" {
		jsonOut.Description = result.Description
	}

	jsonOut.Tags = make(map[string]string)
	for _, tag := range result.Tags {
		jsonOut.Tags[tag.Key] = tag.Value
	}

	return output.WriteJSON(stdout, jsonOut)
}

// ShowCommand returns the SSM Parameter Store show command.
func ShowCommand() *cli.Command {
	return generic.ShowCommand(generic.ShowConfig[*version.NumericSpec]{
		Usage:     "Show parameter value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a parameter's value along with its metadata (name, version, type, modification date).

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago (e.g., ~1, ~2); ~ alone means ~1

EXAMPLES:
  suve aws param show /app/config                               Show latest version
  suve aws param show /app/config~                              Show previous version
  suve aws param show /app/config#3                             Show version 3
  suve aws param show --raw /app/config                         Output raw value (for piping)
  suve aws param show --parse-json /app/config                  Pretty print JSON value
  suve aws param show --output=json /app/config                 Output as JSON
  DB_URL=$(suve aws param show --raw /app/config)               Use in shell variable`,
		UsageError: "usage: suve aws param show <name>",
		ParseSpec:  version.AWSParameterStore.Parse,
		NewPresenter: func(ctx context.Context, spec *version.NumericSpec) (generic.ShowPresenter, error) {
			store, err := awsinternal.ParamStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewShowPresenter(store, spec), nil
		},
	})
}
