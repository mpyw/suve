// Package delete provides the SSM Parameter Store delete command.
package delete

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the delete command.
type Runner struct {
	UseCase *param.DeleteUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the delete command.
type Options struct {
	Name string
}

// Command returns the delete command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Aliases:   []string{"rm"},
		Usage:     "Delete parameter",
		ArgsUsage: "<name>",
		Description: `Permanently delete a parameter from AWS Systems Manager Parameter Store.

WARNING: This action is irreversible. The parameter and all its version
history will be permanently deleted.

EXAMPLES:
   suve param delete /app/config/old-param       Delete a parameter (with confirmation)
   suve param delete --yes /app/config/old-param Delete without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return errors.New("usage: suve param delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// Get AWS identity for confirmation display
	var identity *infra.AWSIdentity
	if !skipConfirm {
		identity, _ = infra.GetAWSIdentity(ctx)
	}

	useCase := &param.DeleteUseCase{Client: client}

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
	}
	if identity != nil {
		prompter.AccountID = identity.AccountID
		prompter.Region = identity.Region
		prompter.Profile = identity.Profile
	}

	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &Runner{
		UseCase: useCase,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name: name,
	})
}

// Run executes the delete command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.DeleteInput{
		Name: opts.Name,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Deleted %s", result.Name)

	return nil
}
