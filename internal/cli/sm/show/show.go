// Package show provides the SM show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the show command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION | :LABEL][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm show my-secret                 Show current version
  suve sm show my-secret~                Show previous version
  suve sm show my-secret:AWSPREVIOUS     Show AWSPREVIOUS label
  suve sm show my-secret:AWSPREVIOUS~1   Show 1 before AWSPREVIOUS
  suve sm show -j my-secret              Pretty print JSON value`,
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

// Run executes the show command.
// Output goes to w, warnings go to errW (typically stderr).
func Run(ctx context.Context, client Client, w io.Writer, errW io.Writer, spec *smversion.Spec, jsonFormat bool) error {
	secret, err := smversion.GetSecretWithVersion(ctx, client, spec)
	if err != nil {
		return err
	}

	out := output.New(w)
	out.Field("Name", aws.ToString(secret.Name))
	out.Field("ARN", aws.ToString(secret.ARN))
	if secret.VersionId != nil {
		out.Field("VersionId", aws.ToString(secret.VersionId))
	}
	if len(secret.VersionStages) > 0 {
		out.Field("Stages", fmt.Sprintf("%v", secret.VersionStages))
	}
	if secret.CreatedDate != nil {
		out.Field("Created", secret.CreatedDate.Format(time.RFC3339))
	}
	out.Separator()

	value := aws.ToString(secret.SecretString)

	// Warn if --json is used but value is not valid JSON
	if jsonFormat {
		if !jsonutil.IsJSON(value) {
			output.Warning(errW, "--json has no effect: value is not valid JSON")
		} else {
			value = jsonutil.Format(value)
		}
	}
	out.Value(value)

	return nil
}
