// Package diff provides the Secrets Manager diff command for comparing secret versions.
//
// The diff command supports multiple argument formats:
//   - Full Spec: Both arguments include name and version (e.g., secret:AWSPREVIOUS secret:AWSCURRENT)
//   - Full Spec single: One full spec compared against AWSCURRENT (e.g., secret:AWSPREVIOUS)
//   - Mixed: First arg with version, second is specifier only (e.g., secret:AWSPREVIOUS ':AWSCURRENT')
//   - Partial Spec: Name followed by specifiers (e.g., secret ':AWSPREVIOUS' ':AWSCURRENT')
//
// When comparing identical versions, a warning and hints are displayed instead of empty diff.
package diff

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// Client is the interface for the diff command.
type Client interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIdsAPI
}

// Runner executes the diff command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	Spec1     *secretversion.Spec
	Spec2     *secretversion.Spec
	ParseJSON bool
	NoPager   bool
	Output    output.Format
}

// JSONOutput represents the JSON output structure for the diff command.
type JSONOutput struct {
	OldName      string `json:"oldName"`
	OldVersionID string `json:"oldVersionId"`
	OldValue     string `json:"oldValue"`
	NewName      string `json:"newName"`
	NewVersionID string `json:"newVersionId"`
	NewValue     string `json:"newValue"`
	Identical    bool   `json:"identical"`
	Diff         string `json:"diff,omitempty"`
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> <version1> [version2]",
		Description: `Compare two versions of a secret in unified diff format.
If only one version/spec is specified, compares against AWSCURRENT.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS)
  ~SHIFT    N versions ago; ~ alone means ~1

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
  suve secret diff my-secret:AWSPREVIOUS my-secret:AWSCURRENT  Compare labels (full spec)
  suve secret diff my-secret:AWSPREVIOUS                       Compare with current (full spec)
  suve secret diff my-secret:AWSPREVIOUS ':AWSCURRENT'         Compare labels (mixed)
  suve secret diff my-secret ':AWSPREVIOUS' ':AWSCURRENT'      Compare labels (partial spec)
  suve secret diff my-secret '~'                               Compare previous with current
  suve secret diff -j my-secret:AWSPREVIOUS                    JSON format before diffing
  suve secret diff --output=json my-secret:AWSPREVIOUS         Output as JSON

For comparing staged values, use: suve stage secret diff`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	spec1, spec2, err := secretversion.ParseDiffArgs(cmd.Args().Slice())
	if err != nil {
		return err
	}

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec1:     spec1,
		Spec2:     spec2,
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
		Output:    output.ParseFormat(cmd.String("output")),
	}

	// JSON output disables pager
	noPager := opts.NoPager || opts.Output == output.FormatJSON

	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		r := &Runner{
			Client: client,
			Stdout: w,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	secret1, err := secretversion.GetSecretWithVersion(ctx, r.Client, opts.Spec1)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	secret2, err := secretversion.GetSecretWithVersion(ctx, r.Client, opts.Spec2)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := lo.FromPtr(secret1.SecretString)
	value2 := lo.FromPtr(secret2.SecretString)

	// Format as JSON if enabled
	if opts.ParseJSON {
		value1, value2 = jsonutil.TryFormatOrWarn2(value1, value2, r.Stderr, "")
	}

	v1 := lo.FromPtr(secret1.VersionId)
	v2 := lo.FromPtr(secret2.VersionId)
	identical := value1 == value2

	// JSON output mode
	if opts.Output == output.FormatJSON {
		jsonOut := JSONOutput{
			OldName:      opts.Spec1.Name,
			OldVersionID: v1,
			OldValue:     value1,
			NewName:      opts.Spec2.Name,
			NewVersionID: v2,
			NewValue:     value2,
			Identical:    identical,
		}
		if !identical {
			jsonOut.Diff = output.DiffRaw(
				fmt.Sprintf("%s#%s", opts.Spec1.Name, secretversion.TruncateVersionID(v1)),
				fmt.Sprintf("%s#%s", opts.Spec2.Name, secretversion.TruncateVersionID(v2)),
				value1,
				value2,
			)
		}
		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonOut)
	}

	if identical {
		output.Warning(r.Stderr, "comparing identical versions")
		output.Hint(r.Stderr, "To compare with previous version, use: suve secret diff %s~1", opts.Spec1.Name)
		output.Hint(r.Stderr, "or: suve secret diff %s:AWSPREVIOUS", opts.Spec1.Name)
		return nil
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%s", opts.Spec1.Name, secretversion.TruncateVersionID(v1)),
		fmt.Sprintf("%s#%s", opts.Spec2.Name, secretversion.TruncateVersionID(v2)),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}
