// Package cat provides the SM cat command.
package cat

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the cat command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Command returns the cat command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "cat",
		Usage:     "Output raw secret value (for piping)",
		ArgsUsage: "<name[#id | :label][shifts]>",
		Description: `Output the raw secret value without any formatting.
Does not append a trailing newline. Designed for scripts and piping.

VERSION SPECIFIERS:
  #ID     Specific version by VersionId
  :LABEL  Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~N      N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm cat my-secret              Output current value
  suve sm cat my-secret~             Output previous version
  suve sm cat my-secret:AWSPREVIOUS  Output AWSPREVIOUS label
  API_KEY=$(suve sm cat my-api-key)  Use in shell variable`,
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	spec, err := smversion.Parse(c.Args().First())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, spec)
}

// Run executes the cat command.
func Run(ctx context.Context, client Client, w io.Writer, spec *smversion.Spec) error {
	secret, err := smversion.GetSecretWithVersion(ctx, client, spec)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(w, aws.ToString(secret.SecretString))
	return nil
}
