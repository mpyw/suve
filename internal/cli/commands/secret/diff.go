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
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// diffJSONOutput represents the JSON output structure for the diff command.
type diffJSONOutput struct {
	OldName      string `json:"oldName"`
	OldVersionID string `json:"oldVersionId"`
	OldValue     string `json:"oldValue"`
	NewName      string `json:"newName"`
	NewVersionID string `json:"newVersionId"`
	NewValue     string `json:"newValue"`
	Identical    bool   `json:"identical"`
	Diff         string `json:"diff,omitempty"`
}

// diffPresenter renders Secrets Manager diff output byte-for-byte as before.
type diffPresenter struct {
	uc     *secret.DiffUseCase
	spec1  *secretversion.Spec
	spec2  *secretversion.Spec
	result *secret.DiffOutput
}

// NewDiffPresenter builds a secret diff presenter over the given reader and specs.
// It is exported for the shared golden-output test harness.
func NewDiffPresenter(reader provider.Reader, spec1, spec2 *secretversion.Spec) genericdiff.Presenter {
	return &diffPresenter{uc: &secret.DiffUseCase{Reader: reader}, spec1: spec1, spec2: spec2}
}

func (p *diffPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, secret.DiffInput{Spec1: p.spec1, Spec2: p.spec2})
	if err != nil {
		return err
	}

	p.result = result

	return nil
}

func (p *diffPresenter) OldValue() string { return p.result.OldValue }
func (p *diffPresenter) NewValue() string { return p.result.NewValue }

func (p *diffPresenter) Labels() (string, string) {
	return fmt.Sprintf("%s#%s", p.result.OldName, secretversion.TruncateVersionID(p.result.OldVersionID)),
		fmt.Sprintf("%s#%s", p.result.NewName, secretversion.TruncateVersionID(p.result.NewVersionID))
}

func (p *diffPresenter) RenderJSON(stdout io.Writer, oldValue, newValue string, identical bool, diff string) error {
	jsonOut := diffJSONOutput{
		OldName:      p.result.OldName,
		OldVersionID: p.result.OldVersionID,
		OldValue:     oldValue,
		NewName:      p.result.NewName,
		NewVersionID: p.result.NewVersionID,
		NewValue:     newValue,
		Identical:    identical,
		Diff:         diff,
	}

	return output.WriteJSON(stdout, jsonOut)
}

func (p *diffPresenter) Hints(stderr io.Writer) {
	output.Hint(stderr, "To compare with previous version, use: suve secret diff %s~1", p.result.OldName)
	output.Hint(stderr, "or: suve secret diff %s:AWSPREVIOUS", p.result.OldName)
}

// DiffCommand returns the Secrets Manager diff command.
func DiffCommand() *cli.Command {
	return genericdiff.Command(genericdiff.Config[*secretversion.Spec]{
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> #<version1> [#<version2>]",
		Description: `Compare two versions of a secret in unified diff format.
If only one version/spec is specified, compares against AWSCURRENT.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS)
  ~SHIFT    N versions ago; ~ alone means ~1

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
  suve secret diff my-secret~                        Compare previous with current
  suve secret diff my-secret:AWSPREVIOUS             Compare AWSPREVIOUS with AWSCURRENT
  suve secret diff my-secret#abc my-secret#def       Compare specific version IDs
  suve secret diff --parse-json my-secret~           Format JSON values before diffing
  suve secret diff --output=json my-secret~          Output comparison as JSON

For comparing staged values, use: suve stage secret diff`,
		ParseDiffArgs: secretversion.ParseDiffArgs,
		NewPresenter: func(ctx context.Context, spec1, spec2 *secretversion.Spec) (genericdiff.Presenter, error) {
			store, err := cliinternal.SecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewDiffPresenter(store, spec1, spec2), nil
		},
	})
}
