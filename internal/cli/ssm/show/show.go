// Package show provides the SSM show command.
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
	"github.com/mpyw/suve/internal/ssm"
	"github.com/mpyw/suve/internal/version"
)

// Client is the interface for the show command.
type Client interface {
	ssm.GetParameterAPI
	ssm.GetParameterHistoryAPI
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show parameter value with metadata",
		ArgsUsage: "<name[@version][~shift]>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "decrypt",
				Aliases: []string{"d"},
				Value:   true,
				Usage:   "Decrypt SecureString values",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	client, err := internalaws.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, spec, c.Bool("decrypt"))
}

// Run executes the show command.
func Run(ctx context.Context, client Client, w io.Writer, spec *version.Spec, decrypt bool) error {
	param, err := ssm.GetParameterWithVersion(ctx, client, spec, decrypt)
	if err != nil {
		return err
	}

	out := output.New(w)
	out.Field("Name", aws.ToString(param.Name))
	out.Field("Version", fmt.Sprintf("%d", param.Version))
	out.Field("Type", string(param.Type))
	if param.LastModifiedDate != nil {
		out.Field("Modified", param.LastModifiedDate.Format(time.RFC3339))
	}
	out.Separator()
	out.Value(aws.ToString(param.Value))

	return nil
}
