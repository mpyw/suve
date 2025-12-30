package sm

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
		Usage:     "Output raw secret value (for piping)",
		ArgsUsage: "<name[@version][~shift][:label]>",
		Action:    catAction,
	}
}

func catAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Cat(ctx, client, c.App.Writer, spec)
}

// Cat outputs raw secret value.
func Cat(ctx context.Context, client CatClient, w io.Writer, spec *version.Spec) error {
	secret, err := GetSecretWithVersion(ctx, client, spec)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(w, aws.ToString(secret.SecretString))
	return nil
}
