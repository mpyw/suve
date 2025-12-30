package ssm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/version"
)

func catCommand() *cli.Command {
	return &cli.Command{
		Name:      "cat",
		Usage:     "Output raw parameter value (for piping)",
		ArgsUsage: "<name[@version][~shift]>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "decrypt",
				Aliases: []string{"d"},
				Value:   true,
				Usage:   "Decrypt SecureString values",
			},
		},
		Action: catAction,
	}
}

func catAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Cat(ctx, client, c.App.Writer, spec, c.Bool("decrypt"))
}

// Cat outputs raw parameter value.
func Cat(ctx context.Context, client CatClient, w io.Writer, spec *version.Spec, decrypt bool) error {
	param, err := GetParameterWithVersion(ctx, client, spec, decrypt)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(w, aws.ToString(param.Value))
	return nil
}
