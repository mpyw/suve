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
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
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
		ArgsUsage: "<name[#VERSION | :LABEL][~SHIFT]*>",
		Description: `Output the raw secret value without any formatting.
Does not append a trailing newline. Designed for scripts and piping.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm cat my-secret              Output current value
  suve sm cat my-secret~             Output previous version
  suve sm cat my-secret:AWSPREVIOUS  Output AWSPREVIOUS label
  suve sm cat -j my-secret           Pretty print JSON value
  API_KEY=$(suve sm cat my-api-key)  Use in shell variable`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values (keys are always sorted alphabetically)",
			},
		},
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

	return Run(c.Context, client, c.App.Writer, c.App.ErrWriter, spec, c.Bool("json"))
}

// Run executes the cat command.
// Output goes to w, warnings go to warnW (typically stderr).
func Run(ctx context.Context, client Client, w io.Writer, warnW io.Writer, spec *smversion.Spec, jsonFormat bool) error {
	secret, err := smversion.GetSecretWithVersion(ctx, client, spec)
	if err != nil {
		return err
	}

	value := aws.ToString(secret.SecretString)

	// Warn if --json is used but value is not valid JSON
	if jsonFormat {
		if !jsonutil.IsJSON(value) {
			output.Warning(warnW, "--json has no effect: value is not valid JSON")
		} else {
			value = jsonutil.Format(value)
		}
	}

	_, _ = fmt.Fprint(w, value)
	return nil
}
