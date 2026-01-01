// Package diff provides the SM diff command for comparing secret versions.
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
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/diff"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/smutil"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the diff command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the diff command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	Spec1      *smversion.Spec
	Spec2      *smversion.Spec
	JSONFormat bool
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

EXAMPLES:
  suve sm diff my-secret:AWSPREVIOUS my-secret:AWSCURRENT  Compare labels (full spec)
  suve sm diff my-secret:AWSPREVIOUS                       Compare with current (full spec)
  suve sm diff my-secret:AWSPREVIOUS ':AWSCURRENT'         Compare labels (mixed)
  suve sm diff my-secret ':AWSPREVIOUS' ':AWSCURRENT'      Compare labels (partial spec)
  suve sm diff my-secret '~'                               Compare previous with current
  suve sm diff -j my-secret:AWSPREVIOUS                    JSON format before diffing`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	spec1, spec2, err := diff.ParseArgs(
		cmd.Args().Slice(),
		smversion.Parse,
		func(abs smversion.AbsoluteSpec) bool { return abs.ID != nil || abs.Label != nil },
		"#:~",
		"usage: suve sm diff <spec1> [spec2] | <name> <version1> [version2]",
	)
	if err != nil {
		return err
	}

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Spec1:      spec1,
		Spec2:      spec2,
		JSONFormat: cmd.Bool("json"),
	})
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	secret1, err := smversion.GetSecretWithVersion(ctx, r.Client, opts.Spec1)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	secret2, err := smversion.GetSecretWithVersion(ctx, r.Client, opts.Spec2)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := lo.FromPtr(secret1.SecretString)
	value2 := lo.FromPtr(secret2.SecretString)

	// Format as JSON if enabled
	if opts.JSONFormat {
		formatted1, ok1 := jsonutil.TryFormat(value1)
		formatted2, ok2 := jsonutil.TryFormat(value2)
		if ok1 && ok2 {
			value1 = formatted1
			value2 = formatted2
		} else {
			output.Warning(r.Stderr, "--json has no effect: some values are not valid JSON")
		}
	}

	if value1 == value2 {
		output.Warning(r.Stderr, "comparing identical versions")
		output.Hint(r.Stderr, "To compare with previous version, use: suve sm diff %s~1", opts.Spec1.Name)
		output.Hint(r.Stderr, "or: suve sm diff %s:AWSPREVIOUS", opts.Spec1.Name)
		return nil
	}

	v1 := smutil.TruncateVersionID(lo.FromPtr(secret1.VersionId))
	v2 := smutil.TruncateVersionID(lo.FromPtr(secret2.VersionId))

	diff := output.Diff(
		fmt.Sprintf("%s#%s", opts.Spec1.Name, v1),
		fmt.Sprintf("%s#%s", opts.Spec2.Name, v2),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}
