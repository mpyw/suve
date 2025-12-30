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
	"github.com/mpyw/suve/internal/version"
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
		ArgsUsage: "<name[@version][~shift][:label]>",
		Description: `Display a secret's value along with its metadata (name, ARN, version ID,
staging labels, creation date).

VERSION SPECIFIERS:
   @ID     Specific version by VersionId (e.g., @abc12345-...)
   ~N      Relative version (e.g., ~1 for previous version)
   :LABEL  Staging label (AWSCURRENT, AWSPREVIOUS, or custom)

STAGING LABELS:
   AWSCURRENT   The current active version (default)
   AWSPREVIOUS  The previous version before the last rotation

EXAMPLES:
   suve sm show my-secret                   Show current version
   suve sm show my-secret:AWSPREVIOUS       Show previous version by label
   suve sm show my-secret~1                 Show previous version by shift
   suve sm show my-secret@abc12345          Show specific version by ID
   suve sm show -j my-secret                Pretty print JSON secret value`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, spec, c.Bool("json"))
}

// Run executes the show command.
func Run(ctx context.Context, client Client, w io.Writer, spec *version.Spec, prettyJSON bool) error {
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
	if prettyJSON {
		value = jsonutil.Format(value)
	}
	out.Value(value)

	return nil
}
