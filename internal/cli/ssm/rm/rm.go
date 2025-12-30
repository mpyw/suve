// Package rm provides the SSM rm command.
package rm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the rm command.
type Client interface {
	ssmapi.DeleteParameterAPI
}

// Command returns the rm command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "Delete parameter",
		ArgsUsage: "<name>",
		Action:    action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name)
}

// Run executes the rm command.
func Run(ctx context.Context, client Client, w io.Writer, name string) error {
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
