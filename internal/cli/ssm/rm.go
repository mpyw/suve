package ssm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
)

func rmCommand() *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "Delete parameter",
		ArgsUsage: "<name>",
		Action:    rmAction,
	}
}

func rmAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()

	ctx := context.Background()
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Rm(ctx, client, c.App.Writer, name)
}

// Rm deletes a parameter.
func Rm(ctx context.Context, client RmClient, w io.Writer, name string) error {
	_, err := client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	red := color.New(color.FgRed).SprintFunc()
	_, _ = fmt.Fprintf(w, "%s %s\n", red("Deleted"), name)

	return nil
}
