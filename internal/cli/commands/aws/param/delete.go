package param

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/param"
)

// DeleteRunner executes the delete command.
type DeleteRunner struct {
	UseCase *param.DeleteUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// DeleteOptions holds the options for the delete command.
type DeleteOptions struct {
	Name string
}

// DeleteCommand returns the delete command.
func DeleteCommand() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Aliases:   []string{"rm"},
		Usage:     "Delete parameter",
		ArgsUsage: "<name>",
		Description: `Permanently delete a parameter from AWS Systems Manager Parameter Store.

WARNING: This action is irreversible. The parameter and all its version
history will be permanently deleted.

EXAMPLES:
   suve aws param delete /app/config/old-param       Delete a parameter (with confirmation)
   suve aws param delete --yes /app/config/old-param Delete without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: deleteAction,
	}
}

func deleteAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return errors.New("usage: suve aws param delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	store, err := awsinternal.ParamStore(ctx)
	if err != nil {
		return err
	}

	// Resolve the AWS target for the confirmation display
	var target string
	if !skipConfirm {
		target = awsinternal.ConfirmTarget(ctx)
	}

	useCase := &param.DeleteUseCase{Store: store}

	// Show current value before confirming
	if !skipConfirm {
		currentValue, _ := useCase.GetCurrentValue(ctx, name)
		if currentValue != "" {
			output.Info(cmd.Root().ErrWriter, "Current value of %s:", name)
			output.Println(cmd.Root().ErrWriter, "")
			output.Println(cmd.Root().ErrWriter, output.Indent(currentValue, "  "))
			output.Println(cmd.Root().ErrWriter, "")
		}
	}

	// Confirm deletion
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
		Target: target,
	}

	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &DeleteRunner{
		UseCase: useCase,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, DeleteOptions{
		Name: name,
	})
}

// Run executes the delete command.
func (r *DeleteRunner) Run(ctx context.Context, opts DeleteOptions) error {
	result, err := r.UseCase.Execute(ctx, param.DeleteInput{
		Name: opts.Name,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Deleted %s", result.Name)

	return nil
}
