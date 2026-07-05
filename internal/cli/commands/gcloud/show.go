package gcloud

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
	"github.com/mpyw/suve/internal/usecase/gcloud"
	"github.com/mpyw/suve/internal/version/gcloudversion"
)

// showJSONOutput represents the JSON output structure for the show command.
type showJSONOutput struct {
	Name    string            `json:"name"`
	Version string            `json:"version,omitempty"`
	Created string            `json:"created,omitempty"`
	Labels  map[string]string `json:"labels"`
	Value   string            `json:"value"`
}

// showPresenter renders Google Cloud Secret Manager show output.
type showPresenter struct {
	uc     *gcloud.ShowUseCase
	spec   *gcloudversion.Spec
	result *gcloud.ShowOutput
}

// NewShowPresenter builds a Google Cloud show presenter over the given reader and spec.
func NewShowPresenter(reader provider.Reader, spec *gcloudversion.Spec) genericshow.Presenter {
	return &showPresenter{uc: &gcloud.ShowUseCase{Reader: reader}, spec: spec}
}

func (p *showPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, gcloud.ShowInput{Spec: p.spec})
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
		Name:    result.Name,
		Version: result.Version,
		Value:   value,
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
	return genericshow.Command(genericshow.Config[*gcloudversion.Spec]{
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

VERSION SPECIFIERS:
  #VERSION  Specific version by integer number
  ~SHIFT    N enabled versions ago; ~ alone means ~1

EXAMPLES:
  suve gcloud secret show my-secret                        Show latest version
  suve gcloud secret show my-secret#3                      Show version 3
  suve gcloud secret show my-secret~                       Show previous version
  suve gcloud secret show --raw my-secret                  Output raw value (for piping)
  suve gcloud secret show --output=json my-secret          Output as JSON`,
		UsageError: "usage: suve gcloud secret show <name>",
		ParseSpec:  gcloudversion.Parse,
		NewPresenter: func(ctx context.Context, spec *gcloudversion.Spec) (genericshow.Presenter, error) {
			store, err := cliinternal.GoogleCloudSecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewShowPresenter(store, spec), nil
		},
	})
}
