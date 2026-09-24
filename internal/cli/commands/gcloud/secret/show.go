package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	gcloudinternal "github.com/mpyw/suve/internal/cli/commands/gcloud/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version"
)

// showJSONOutput represents the JSON output structure for the show command.
type showJSONOutput struct {
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	State       string            `json:"state,omitempty"`
	Description string            `json:"description,omitempty"`
	Created     string            `json:"created,omitempty"`
	Labels      map[string]string `json:"labels"`
	Value       string            `json:"value"`
}

// showPresenter renders Google Cloud Secret Manager show output.
type showPresenter struct {
	uc     *secret.ShowUseCase
	spec   *version.NumericSpec
	result *secret.ShowOutput
}

// NewShowPresenter builds a Google Cloud show presenter over the given reader and spec.
func NewShowPresenter(reader provider.Reader, spec *version.NumericSpec) generic.ShowPresenter {
	return &showPresenter{uc: &secret.ShowUseCase{Reader: reader}, spec: spec}
}

func (p *showPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, secret.ShowInput{Name: p.spec.Name, Suffix: version.GoogleCloudSecretManager.Suffix(p.spec)})
	if err != nil {
		return err
	}

	p.result = result

	return nil
}

func (p *showPresenter) Value(parseJSON bool, stderr io.Writer) string {
	value := p.result.Value
	if parseJSON {
		value = jsonutil.TryFormatOrWarn(value, stderr, "")
	}

	return value
}

func (p *showPresenter) RenderText(stdout io.Writer, value string) {
	result := p.result

	out := output.New(stdout)
	out.Field("Name", result.Name)

	if result.Version != "" {
		out.Field("Version", result.Version)
	}

	if result.State != "" {
		out.Field("State", result.State)
	}

	if result.Description != "" {
		out.Field("Description", result.Description)
	}

	if result.CreatedDate != nil {
		out.Field("Created", timeutil.FormatRFC3339(*result.CreatedDate))
	}

	if len(result.Tags) > 0 {
		out.Field("Labels", fmt.Sprintf("%d label(s)", len(result.Tags)))

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
		Name:        result.Name,
		Version:     result.Version,
		State:       result.State,
		Description: result.Description,
		Value:       value,
	}

	if result.CreatedDate != nil {
		jsonOut.Created = timeutil.FormatRFC3339(*result.CreatedDate)
	}

	jsonOut.Labels = make(map[string]string)
	for _, tag := range result.Tags {
		jsonOut.Labels[tag.Key] = tag.Value
	}

	return output.WriteJSON(stdout, jsonOut)
}

// ShowCommand returns the Google Cloud Secret Manager show command.
func ShowCommand() *cli.Command {
	return generic.ShowCommand(generic.ShowConfig[*version.NumericSpec]{
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

VERSION SPECIFIERS:
  #VERSION  Specific version by integer number
  ~SHIFT    N versions ago (any state); ~ alone means ~1

EXAMPLES:
  suve gcloud secret show my-secret                        Show latest version
  suve gcloud secret show my-secret#3                      Show version 3
  suve gcloud secret show my-secret~                       Show previous version
  suve gcloud secret show --raw my-secret                  Output raw value (for piping)
  suve gcloud secret show --output=json my-secret          Output as JSON`,
		UsageError: "usage: suve gcloud secret show <name>",
		ParseSpec:  version.GoogleCloudSecretManager.Parse,
		NewPresenter: func(ctx context.Context, spec *version.NumericSpec) (generic.ShowPresenter, error) {
			store, err := gcloudinternal.SecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewShowPresenter(store, spec), nil
		},
	})
}
