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
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azurekvversion"
)

// showJSONOutput represents the JSON output structure for the show command.
type showJSONOutput struct {
	Name    string            `json:"name"`
	Version string            `json:"version,omitempty"`
	Created string            `json:"created,omitempty"`
	Tags    map[string]string `json:"tags"`
	Value   string            `json:"value"`
}

// showPresenter renders Azure Key Vault show output.
type showPresenter struct {
	uc     *azure.ShowUseCase
	spec   *azurekvversion.Spec
	result *azure.ShowOutput
}

// NewShowPresenter builds an Azure Key Vault show presenter over the given reader and spec.
func NewShowPresenter(reader provider.Reader, spec *azurekvversion.Spec) genericshow.Presenter {
	return &showPresenter{uc: &azure.ShowUseCase{Reader: reader}, spec: spec}
}

func (p *showPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, azure.ShowInput{Name: p.spec.Name, Suffix: specSuffix(p.spec)})
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
		Version: result.Version,
		Value:   value,
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

// ShowCommand returns the Azure Key Vault show command.
func ShowCommand() *cli.Command {
	return genericshow.Command(genericshow.Config[*azurekvversion.Spec]{
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

Use --raw to output only the value without metadata (for piping/scripting).
Use --output=json for structured JSON output (cannot be used with --raw).

VERSION SPECIFIERS:
  #VERSION  Specific version by opaque id
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve azure secret show my-secret                        Show current version
  suve azure secret show my-secret#abc123                 Show a specific version id
  suve azure secret show my-secret~                       Show previous version
  suve azure secret show --raw my-secret                  Output raw value (for piping)
  suve azure secret show --output=json my-secret          Output as JSON`,
		UsageError: "usage: suve azure secret show <name>",
		ParseSpec:  azurekvversion.Parse,
		NewPresenter: func(ctx context.Context, spec *azurekvversion.Spec) (genericshow.Presenter, error) {
			store, err := cliinternal.AzureKeyVaultStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewShowPresenter(store, spec), nil
		},
	})
}
