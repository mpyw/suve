package param

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"

	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// diffJSONOutput represents the JSON output structure for the diff command.
// App Configuration is unversioned, so no version fields are emitted.
type diffJSONOutput struct {
	OldName   string `json:"oldName"`
	OldValue  string `json:"oldValue"`
	NewName   string `json:"newName"`
	NewValue  string `json:"newValue"`
	Identical bool   `json:"identical"`
	Diff      string `json:"diff,omitempty"`
}

// diffPresenter renders Azure App Configuration diff output. Since App
// Configuration is unversioned, diff compares two settings (by key) rather than
// two versions of one key.
type diffPresenter struct {
	uc     *azure.DiffUseCase
	spec1  *azureappconfigversion.Spec
	spec2  *azureappconfigversion.Spec
	result *azure.DiffOutput
}

// NewDiffPresenter builds an Azure App Configuration diff presenter over the given reader and specs.
func NewDiffPresenter(reader provider.Reader, spec1, spec2 *azureappconfigversion.Spec) genericdiff.Presenter {
	return &diffPresenter{uc: &azure.DiffUseCase{Reader: reader}, spec1: spec1, spec2: spec2}
}

func (p *diffPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, azure.DiffInput{
		Name1:   p.spec1.Name,
		Suffix1: "",
		Name2:   p.spec2.Name,
		Suffix2: "",
	})
	if err != nil {
		return err
	}

	p.result = result

	return nil
}

func (p *diffPresenter) OldValue() string { return p.result.OldValue }
func (p *diffPresenter) NewValue() string { return p.result.NewValue }

func (p *diffPresenter) Labels() (string, string) {
	return p.result.OldName, p.result.NewName
}

func (p *diffPresenter) RenderJSON(stdout io.Writer, oldValue, newValue string, identical bool, diff string) error {
	jsonOut := diffJSONOutput{
		OldName:   p.result.OldName,
		OldValue:  oldValue,
		NewName:   p.result.NewName,
		NewValue:  newValue,
		Identical: identical,
		Diff:      diff,
	}

	return output.WriteJSON(stdout, jsonOut)
}

func (p *diffPresenter) Hints(stderr io.Writer) {
	output.Hint(stderr, "App Configuration is unversioned; compare two distinct keys, e.g.: suve azure param diff key-a key-b")
}

// DiffCommand returns the Azure App Configuration diff command.
func DiffCommand() *cli.Command {
	return genericdiff.Command(genericdiff.Config[*azureappconfigversion.Spec]{
		Usage:     "Show diff between two settings",
		ArgsUsage: "<key1> [key2]",
		Description: `Compare two App Configuration settings in unified diff format.

App Configuration is UNVERSIONED, so diff compares two distinct keys rather than
two versions of one key. #, ~, and : in a key are literal characters, not
version specifiers.

EXAMPLES:
  suve azure param diff key-a key-b                    Compare two settings
  suve azure param diff --output=json key-a key-b      Output comparison as JSON`,
		ParseDiffArgs: azureappconfigversion.ParseDiffArgs,
		NewPresenter: func(ctx context.Context, spec1, spec2 *azureappconfigversion.Spec) (genericdiff.Presenter, error) {
			store, err := cliinternal.AzureAppConfigStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewDiffPresenter(store, spec1, spec2), nil
		},
	})
}
