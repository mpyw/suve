package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/secretsmanager"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version"
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
	spec1  *version.OpaqueSpec
	spec2  *version.OpaqueSpec
	result *secret.DiffOutput
}

// NewDiffPresenter builds a secret diff presenter over the given reader and specs.
// It is exported for the shared golden-output test harness.
func NewDiffPresenter(reader provider.Reader, spec1, spec2 *version.OpaqueSpec) generic.DiffPresenter {
	return &diffPresenter{uc: &secret.DiffUseCase{Reader: reader}, spec1: spec1, spec2: spec2}
}

func (p *diffPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, secret.DiffInput{
		Name1: p.spec1.Name, Suffix1: version.SecretsManager.Suffix(p.spec1),
		Name2: p.spec2.Name, Suffix2: version.SecretsManager.Suffix(p.spec2),
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
	return fmt.Sprintf("%s#%s", p.result.OldName, secretsmanager.TruncateVersionID(p.result.OldVersion)),
		fmt.Sprintf("%s#%s", p.result.NewName, secretsmanager.TruncateVersionID(p.result.NewVersion))
}

func (p *diffPresenter) RenderJSON(stdout io.Writer, oldValue, newValue string, identical bool, diff string) error {
	jsonOut := diffJSONOutput{
		OldName:      p.result.OldName,
		OldVersionID: p.result.OldVersion,
		OldValue:     oldValue,
		NewName:      p.result.NewName,
		NewVersionID: p.result.NewVersion,
		NewValue:     newValue,
		Identical:    identical,
		Diff:         diff,
	}

	return output.WriteJSON(stdout, jsonOut)
}

func (p *diffPresenter) Hints(stderr io.Writer) {
	output.Hint(stderr, "To compare with previous version, use: suve aws secret diff %s~1", p.result.OldName)
	output.Hint(stderr, "or: suve aws secret diff %s:AWSPREVIOUS", p.result.OldName)
}

// DiffCommand returns the Secrets Manager diff command.
func DiffCommand() *cli.Command {
	return generic.DiffCommand(generic.DiffConfig[*version.OpaqueSpec]{
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
  suve aws secret diff my-secret~                        Compare previous with current
  suve aws secret diff my-secret:AWSPREVIOUS             Compare AWSPREVIOUS with AWSCURRENT
  suve aws secret diff my-secret#abc my-secret#def       Compare specific version IDs
  suve aws secret diff --parse-json my-secret~           Format JSON values before diffing
  suve aws secret diff --output=json my-secret~          Output comparison as JSON

For comparing staged values, use: suve aws stage secret diff`,
		ParseDiffArgs: parseDiffArgs,
		NewPresenter: func(ctx context.Context, spec1, spec2 *version.OpaqueSpec) (generic.DiffPresenter, error) {
			store, err := awsinternal.SecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewDiffPresenter(store, spec1, spec2), nil
		},
	})
}

// parseDiffArgs parses the diff arguments with the Secrets Manager grammar.
func parseDiffArgs(args []string) (*version.OpaqueSpec, *version.OpaqueSpec, error) {
	return diffargs.ParseArgs(
		args,
		version.SecretsManager.Parse,
		version.OpaqueAbsolute.IsSet,
		"#:~",
		"usage: suve aws secret diff <spec1> [spec2] | <name> #<version1> [#<version2>]",
	)
}
