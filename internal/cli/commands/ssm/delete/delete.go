// Package delete provides the SSM delete command.
package delete

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/infra"
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
   suve ssm delete /app/config/old-param         Delete a parameter (with confirmation)
   suve ssm delete -y /app/config/old-param      Delete without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	// Confirm deletion
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	client, err := infra.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name: name,
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

	_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", colors.OpDelete("Deleted"), opts.Name)

	return nil
}
