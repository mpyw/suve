package param

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// diffJSONOutput represents the JSON output structure for the diff command.
type diffJSONOutput struct {
	OldName    string `json:"oldName"`
	OldVersion int64  `json:"oldVersion"`
	OldValue   string `json:"oldValue"`
	NewName    string `json:"newName"`
	NewVersion int64  `json:"newVersion"`
	NewValue   string `json:"newValue"`
	Identical  bool   `json:"identical"`
	Diff       string `json:"diff,omitempty"`
}

// diffPresenter renders SSM Parameter Store diff output byte-for-byte as before.
type diffPresenter struct {
	uc     *param.DiffUseCase
	spec1  *paramversion.Spec
	spec2  *paramversion.Spec
	result *param.DiffOutput
}

// NewDiffPresenter builds a param diff presenter over the given reader and specs.
// It is exported for the shared golden-output test harness.
func NewDiffPresenter(reader provider.Reader, spec1, spec2 *paramversion.Spec) genericdiff.Presenter {
	return &diffPresenter{uc: &param.DiffUseCase{Reader: reader}, spec1: spec1, spec2: spec2}
}

func (p *diffPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, param.DiffInput{Spec1: p.spec1, Spec2: p.spec2})
	if err != nil {
		return err
	}

	p.result = result

	return nil
}

func (p *diffPresenter) OldValue() string { return p.result.OldValue }
func (p *diffPresenter) NewValue() string { return p.result.NewValue }

func (p *diffPresenter) Labels() (string, string) {
	return fmt.Sprintf("%s#%d", p.result.OldName, p.result.OldVersion),
		fmt.Sprintf("%s#%d", p.result.NewName, p.result.NewVersion)
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
	output.Hint(stderr, "To compare with previous version, use: suve param diff %s~1", p.spec1.Name)
}

// DiffCommand returns the SSM Parameter Store diff command.
func DiffCommand() *cli.Command {
	return genericdiff.Command(genericdiff.Config[*paramversion.Spec]{
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
  suve param diff /app/config~                    Compare previous with latest
  suve param diff /app/config#3                   Compare version 3 with latest
  suve param diff /app/config#1 /app/config#2     Compare version 1 and 2
  suve param diff --parse-json /app/config~       Format JSON values before diffing
  suve param diff --output=json /app/config~      Output comparison as JSON

For comparing staged values, use: suve stage param diff`,
		ParseDiffArgs: paramversion.ParseDiffArgs,
		NewPresenter: func(ctx context.Context, spec1, spec2 *paramversion.Spec) (genericdiff.Presenter, error) {
			client, err := cliinternal.NewParamClient(ctx)
			if err != nil {
				return nil, err
			}

			return NewDiffPresenter(awsparam.New(client), spec1, spec2), nil
		},
	})
}
