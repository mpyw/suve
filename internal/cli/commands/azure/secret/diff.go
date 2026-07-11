package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azurekvversion"
)

// diffJSONOutput represents the JSON output structure for the diff command.
type diffJSONOutput struct {
	OldName    string `json:"oldName"`
	OldVersion string `json:"oldVersion"`
	OldValue   string `json:"oldValue"`
	NewName    string `json:"newName"`
	NewVersion string `json:"newVersion"`
	NewValue   string `json:"newValue"`
	Identical  bool   `json:"identical"`
	Diff       string `json:"diff,omitempty"`
}

// diffPresenter renders Azure Key Vault diff output.
type diffPresenter struct {
	uc     *azure.DiffUseCase
	spec1  *azurekvversion.Spec
	spec2  *azurekvversion.Spec
	result *azure.DiffOutput
}

// NewDiffPresenter builds an Azure Key Vault diff presenter over the given reader and specs.
func NewDiffPresenter(reader provider.Reader, spec1, spec2 *azurekvversion.Spec) genericdiff.Presenter {
	return &diffPresenter{uc: &azure.DiffUseCase{Reader: reader}, spec1: spec1, spec2: spec2}
}

func (p *diffPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, azure.DiffInput{
		Name1:   p.spec1.Name,
		Suffix1: specSuffix(p.spec1),
		Name2:   p.spec2.Name,
		Suffix2: specSuffix(p.spec2),
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
	return fmt.Sprintf("%s#%s", p.result.OldName, p.result.OldVersion),
		fmt.Sprintf("%s#%s", p.result.NewName, p.result.NewVersion)
}

func (p *diffPresenter) RenderJSON(stdout io.Writer, oldValue, newValue string, identical bool, diff string) error {
	jsonOut := diffJSONOutput{
		OldName:    p.result.OldName,
		OldVersion: p.result.OldVersion,
		OldValue:   oldValue,
		NewName:    p.result.NewName,
		NewVersion: p.result.NewVersion,
		NewValue:   newValue,
		Identical:  identical,
		Diff:       diff,
	}

	return output.WriteJSON(stdout, jsonOut)
}

func (p *diffPresenter) Hints(stderr io.Writer) {
	output.Hint(stderr, "To compare with the previous version, use: suve azure secret diff %s~1", p.result.OldName)
}

// DiffCommand returns the Azure Key Vault diff command.
func DiffCommand() *cli.Command {
	return genericdiff.Command(genericdiff.Config[*azurekvversion.Spec]{
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> #<version1> [#<version2>]",
		Description: `Compare two versions of a secret in unified diff format.
If only one version/spec is specified, compares against the current version.

VERSION SPECIFIERS:
  #VERSION  Specific version by opaque id
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve azure secret diff my-secret~                   Compare previous with current
  suve azure secret diff my-secret#abc my-secret#def  Compare two version ids
  suve azure secret diff --parse-json my-secret~      Format JSON values before diffing
  suve azure secret diff --output=json my-secret~     Output comparison as JSON`,
		ParseDiffArgs: azurekvversion.ParseDiffArgs,
		NewPresenter: func(ctx context.Context, spec1, spec2 *azurekvversion.Spec) (genericdiff.Presenter, error) {
			store, err := cliinternal.AzureKeyVaultStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewDiffPresenter(store, spec1, spec2), nil
		},
	})
}
