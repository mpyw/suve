// Package diff provides the SM diff command.
package diff

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the diff command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<name> <version1> [version2]",
		Description: `Compare two versions of a secret in unified diff format.
If only one version is specified, compares against AWSCURRENT.

VERSION SPECIFIERS (as separate arguments):
  #ID     Specific version by VersionId
  :LABEL  Staging label (AWSCURRENT, AWSPREVIOUS)
  ~N      N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm diff my-secret :AWSPREVIOUS :AWSCURRENT  Compare labels
  suve sm diff my-secret :AWSPREVIOUS              Compare with current
  suve sm diff my-secret ~                         Compare previous with current
  suve sm diff my-secret #abc123 #def456           Compare by version ID`,
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm diff <name> <version1> [version2]")
	}

	name := c.Args().Get(0)
	version1 := c.Args().Get(1)
	version2 := c.Args().Get(2)

	if version2 == "" {
		version2 = version1
		version1 = ":AWSCURRENT"
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, version1, version2)
}

// Run executes the diff command.
func Run(ctx context.Context, client Client, w io.Writer, name, version1, version2 string) error {
	spec1, err := smversion.Parse(name + version1)
	if err != nil {
		return fmt.Errorf("invalid version1: %w", err)
	}

	spec2, err := smversion.Parse(name + version2)
	if err != nil {
		return fmt.Errorf("invalid version2: %w", err)
	}

	secret1, err := smversion.GetSecretWithVersion(ctx, client, spec1)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version1, err)
	}

	secret2, err := smversion.GetSecretWithVersion(ctx, client, spec2)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version2, err)
	}

	v1 := aws.ToString(secret1.VersionId)
	if len(v1) > 8 {
		v1 = v1[:8]
	}
	v2 := aws.ToString(secret2.VersionId)
	if len(v2) > 8 {
		v2 = v2[:8]
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%s", name, v1),
		fmt.Sprintf("%s#%s", name, v2),
		aws.ToString(secret1.SecretString),
		aws.ToString(secret2.SecretString),
	)
	_, _ = fmt.Fprint(w, diff)

	return nil
}
