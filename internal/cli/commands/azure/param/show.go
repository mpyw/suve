package param

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
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// showJSONOutput represents the JSON output structure for the show command.
type showJSONOutput struct {
	Name     string            `json:"name"`
	Modified string            `json:"modified,omitempty"`
	Tags     map[string]string `json:"tags"`
	Value    string            `json:"value"`
}

// showPresenter renders Azure App Configuration show output. App Configuration
// is unversioned, so no version/state metadata is rendered.
type showPresenter struct {
	uc     *azure.ShowUseCase
	spec   *azureappconfigversion.Spec
	result *azure.ShowOutput
}

// NewShowPresenter builds an Azure App Configuration show presenter over the given reader and spec.
func NewShowPresenter(reader provider.Reader, spec *azureappconfigversion.Spec) genericshow.Presenter {
	return &showPresenter{uc: &azure.ShowUseCase{Reader: reader}, spec: spec}
}

func (p *showPresenter) Fetch(ctx context.Context) error {
	// App Configuration has no version specifier, so the suffix is always empty.
	result, err := p.uc.Execute(ctx, azure.ShowInput{Name: p.spec.Name, Suffix: ""})
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

	if result.CreatedDate != nil {
		out.Field("Modified", timeutil.FormatRFC3339(*result.CreatedDate))
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
		Value: value,
	}

	if result.CreatedDate != nil {
		jsonOut.Modified = timeutil.FormatRFC3339(*result.CreatedDate)
	}

	jsonOut.Tags = make(map[string]string)
	for _, tag := range result.Tags {
		jsonOut.Tags[tag.Key] = tag.Value
	}

	return output.WriteJSON(stdout, jsonOut)
}

// ShowCommand returns the Azure App Configuration show command.
func ShowCommand() *cli.Command {
	return genericshow.Command(genericshow.Config[*azureappconfigversion.Spec]{
		Usage:     "Show setting value with metadata",
		ArgsUsage: argsUsageKey,
		Description: `Display an App Configuration setting's value along with its metadata.

App Configuration is UNVERSIONED: version specifiers (#VERSION, ~SHIFT, :LABEL)
are rejected with a clear error.

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

EXAMPLES:
  suve azure param show my-key                        Show the setting value
  suve azure param show --raw my-key                  Output raw value (for piping)
  suve azure param show --output=json my-key          Output as JSON`,
		UsageError: "usage: suve azure param show <key>",
		ParseSpec:  azureappconfigversion.Parse,
		NewPresenter: func(ctx context.Context, spec *azureappconfigversion.Spec) (genericshow.Presenter, error) {
			store, err := cliinternal.AzureAppConfigStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewShowPresenter(store, spec), nil
		},
	})
}
