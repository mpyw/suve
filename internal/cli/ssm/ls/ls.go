// Package ls provides the SSM ls command.
package ls

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the ls command.
type Client interface {
	ssmapi.DescribeParametersAPI
}

// Runner executes the ls command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the ls command.
type Options struct {
	Prefix    string
	Recursive bool
}

// Command returns the ls command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "ls",
		Usage:     "List parameters",
		ArgsUsage: "[path-prefix]",
		Description: `List parameters in AWS Systems Manager Parameter Store.

Without a path prefix, lists all parameters in the account.
With a path prefix, lists only parameters under that path.

By default, lists only immediate children of the path.
Use --recursive to include all descendant parameters.

EXAMPLES:
   suve ssm ls                          List all parameters
   suve ssm ls /app                     List parameters directly under /app
   suve ssm ls -r /app                  List all parameters under /app recursively
   suve ssm ls /app/config/             List parameters under /app/config`,
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
	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: c.App.Writer,
		Stderr: c.App.ErrWriter,
	}
	return r.Run(c.Context, Options{
		Prefix:    c.Args().First(),
		Recursive: c.Bool("recursive"),
	})
}

// Run executes the ls command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	option := "OneLevel"
	if opts.Recursive {
		option = "Recursive"
	}

	input := &ssm.DescribeParametersInput{}
	if opts.Prefix != "" {
		input.ParameterFilters = []types.ParameterStringFilter{
			{
				Key:    lo.ToPtr("Path"),
				Option: lo.ToPtr(option),
				Values: []string{opts.Prefix},
			},
		}
	}

	paginator := ssm.NewDescribeParametersPaginator(r.Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, param := range page.Parameters {
			_, _ = fmt.Fprintln(r.Stdout, lo.FromPtr(param.Name))
		}
	}

	return nil
}
