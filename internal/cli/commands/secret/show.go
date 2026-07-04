package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	genericshow "github.com/mpyw/suve/internal/cli/commands/generic/show"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// showJSONOutput represents the JSON output structure for the show command.
type showJSONOutput struct {
	Name      string            `json:"name"`
	ARN       string            `json:"arn"`
	VersionID string            `json:"versionId,omitempty"`
	Stages    []string          `json:"stages,omitempty"`
	Created   string            `json:"created,omitempty"`
	Tags      map[string]string `json:"tags"`
	Value     string            `json:"value"`
}

// showPresenter renders Secrets Manager show output byte-for-byte as before.
type showPresenter struct {
	uc     *secret.ShowUseCase
	spec   *secretversion.Spec
	result *secret.ShowOutput
}

// NewShowPresenter builds a secret show presenter over the given reader and spec.
// It is exported for the shared golden-output test harness.
func NewShowPresenter(reader provider.Reader, spec *secretversion.Spec) genericshow.Presenter {
	return &showPresenter{uc: &secret.ShowUseCase{Reader: reader}, spec: spec}
}

func (p *showPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, secret.ShowInput{Spec: p.spec})
	if err != nil {
		return err
	}

	p.result = result

	return nil
}

func (p *showPresenter) Value(parseJSON bool, stderr io.Writer) string {
	value := p.result.Value

	// Format as JSON if enabled
	if parseJSON {
		value = jsonutil.TryFormatOrWarn(value, stderr, "")
	}

	return value
}

func (p *showPresenter) RenderText(stdout io.Writer, value string) {
	result := p.result

	out := output.New(stdout)
	out.Field("Name", result.Name)
	out.Field("ARN", result.ARN)

	if result.VersionID != "" {
		out.Field("VersionId", result.VersionID)
	}

	if len(result.VersionStage) > 0 {
		out.Field("Stages", fmt.Sprintf("%v", result.VersionStage))
	}

	if result.CreatedDate != nil {
		out.Field("Created", timeutil.FormatRFC3339(*result.CreatedDate))
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
		Name:  result.Name,
		ARN:   result.ARN,
		Value: value,
	}
	if result.VersionID != "" {
		jsonOut.VersionID = result.VersionID
	}

	if len(result.VersionStage) > 0 {
		jsonOut.Stages = result.VersionStage
	}

	if result.CreatedDate != nil {
		jsonOut.Created = timeutil.FormatRFC3339(*result.CreatedDate)
	}

	jsonOut.Tags = make(map[string]string)
	for _, tag := range result.Tags {
		jsonOut.Tags[tag.Key] = tag.Value
	}

	return output.WriteJSON(stdout, jsonOut)
}

// ShowCommand returns the Secrets Manager show command.
func ShowCommand() *cli.Command {
	return genericshow.Command(genericshow.Config[*secretversion.Spec]{
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION | :LABEL][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve secret show my-secret                              Show current version
  suve secret show my-secret~                             Show previous version
  suve secret show my-secret:AWSPREVIOUS                  Show AWSPREVIOUS label
  suve secret show --raw my-secret                        Output raw value (for piping)
  suve secret show --parse-json my-secret                 Pretty print JSON value
  suve secret show --output=json my-secret                Output as JSON
  API_KEY=$(suve secret show --raw my-secret)             Use in shell variable`,
		UsageError: "usage: suve secret show <name>",
		ParseSpec:  secretversion.Parse,
		NewPresenter: func(ctx context.Context, spec *secretversion.Spec) (genericshow.Presenter, error) {
			store, err := cliinternal.SecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewShowPresenter(store, spec), nil
		},
	})
}
