// Package delete provides the SSM delete command.
package delete

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the delete command.
type Client interface {
	ssmapi.DeleteParameterAPI
}

// Runner executes the delete command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the delete command.
type Options struct {
	Name string
}

// Command returns the delete command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete parameter",
		ArgsUsage: "<name>",
		Description: `Permanently delete a parameter from AWS Systems Manager Parameter Store.

WARNING: This action is irreversible. The parameter and all its version
history will be permanently deleted.

EXAMPLES:
   suve ssm delete /app/config/old-param    Delete a parameter`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm delete <name>")
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name: cmd.Args().First(),
	})
}

// Run executes the delete command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	_, err := r.Client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: lo.ToPtr(opts.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	red := color.New(color.FgRed).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", red("Deleted"), opts.Name)

	return nil
}
