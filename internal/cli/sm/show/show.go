// Package show provides the SM show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	internalaws "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/sm"
	"github.com/mpyw/suve/internal/version"
)

// Client is the interface for the show command.
type Client interface {
	sm.GetSecretValueAPI
	sm.ListSecretVersionIdsAPI
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[@version][~shift][:label]>",
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

	ctx := context.Background()
	client, err := internalaws.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(ctx, client, c.App.Writer, spec, c.Bool("json"))
}

// Run executes the show command.
func Run(ctx context.Context, client Client, w io.Writer, spec *version.Spec, prettyJSON bool) error {
	secret, err := sm.GetSecretWithVersion(ctx, client, spec)
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
		value = formatJSON(value)
	}
	out.Value(value)

	return nil
}
