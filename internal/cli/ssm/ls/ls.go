// Package ls provides the SSM ls command.
package ls

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the ls command.
type Client interface {
	ssmapi.DescribeParametersAPI
}

// Command returns the ls command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "ls",
		Usage:     "List parameters",
		ArgsUsage: "[path-prefix]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "recursive",
				Aliases: []string{"r"},
				Usage:   "List recursively",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	prefix := c.Args().First()
	recursive := c.Bool("recursive")

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, prefix, recursive)
}

// Run executes the ls command.
func Run(ctx context.Context, client Client, w io.Writer, prefix string, recursive bool) error {
	option := "OneLevel"
	if recursive {
		option = "Recursive"
	}

	input := &ssm.DescribeParametersInput{}
	if prefix != "" {
		input.ParameterFilters = []types.ParameterStringFilter{
			{
				Key:    aws.String("Path"),
				Option: aws.String(option),
				Values: []string{prefix},
			},
		}
	}

	paginator := ssm.NewDescribeParametersPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, param := range page.Parameters {
			_, _ = fmt.Fprintln(w, aws.ToString(param.Name))
		}
	}

	return nil
}
