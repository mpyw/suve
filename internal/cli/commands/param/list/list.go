// Package list provides the SSM Parameter Store list command.
package list

import (
	"context"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/infra"
)

// Client is the interface for the list command.
type Client interface {
	paramapi.DescribeParametersAPI
}

// Runner executes the list command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the list command.
type Options struct {
	Prefix    string
	Recursive bool
}

// Command returns the list command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List parameters",
		ArgsUsage: "[path-prefix]",
		Description: `List parameters in AWS Systems Manager Parameter Store.

Without a path prefix, lists all parameters in the account.
With a path prefix, lists only parameters under that path.

By default, lists only immediate children of the path.
Use --recursive to include all descendant parameters.

EXAMPLES:
   suve param list                          List all parameters
   suve param list /app                     List parameters directly under /app
   suve param list --recursive /app         List all parameters under /app recursively
   suve param list /app/config/             List parameters under /app/config`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "recursive",
				Aliases: []string{"R"},
				Usage:   "List recursively",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Prefix:    cmd.Args().First(),
		Recursive: cmd.Bool("recursive"),
	})
}

// Run executes the list command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	option := "OneLevel"
	if opts.Recursive {
		option = "Recursive"
	}

	input := &paramapi.DescribeParametersInput{}
	if opts.Prefix != "" {
		input.ParameterFilters = []paramapi.ParameterStringFilter{
			{
				Key:    lo.ToPtr("Path"),
				Option: lo.ToPtr(option),
				Values: []string{opts.Prefix},
			},
		}
	}

	paginator := paramapi.NewDescribeParametersPaginator(r.Client, input)
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
