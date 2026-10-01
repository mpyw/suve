package param

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/param"
)

// deleteRunner executes the delete command.
type deleteRunner struct {
	useCase *param.DeleteUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// deleteOptions holds the options for the delete command.
type deleteOptions struct {
	name string
}

// deleteCommand returns the Azure App Configuration delete command.
//
//declscope:package // command.go registers it
func deleteCommand() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Aliases:   []string{"rm"},
		Usage:     "Delete a setting",
		ArgsUsage: argsUsageKey,
		Description: `Delete a setting (key-value) from Azure App Configuration.

Deletion removes the current value for the key (default label). App
Configuration is unversioned, so there is no prior version to fall back to.

EXAMPLES:
   suve azure param delete app/timeout        Delete (with confirmation)
   suve azure param delete --yes app/timeout  Delete without confirmation`,
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
		return fmt.Errorf("usage: suve azure param delete <key>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	store, err := azureinternal.AppConfigStore(ctx)
	if err != nil {
		return err
	}

	uc := &param.DeleteUseCase{Store: store, ItemNoun: itemNoun()}

	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			output.Info(cmd.Root().ErrWriter, "Current value of %s:", name)
			output.Println(cmd.Root().ErrWriter, "")
			output.Println(cmd.Root().ErrWriter, output.Indent(currentValue, "  "))
			output.Println(cmd.Root().ErrWriter, "")
		}
	}

	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
		Target: azureinternal.AppConfigConfirmTarget(ctx),
	}

	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &deleteRunner{
		useCase: uc,
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, deleteOptions{name: name})
}

// run executes the delete command.
func (r *deleteRunner) run(ctx context.Context, opts deleteOptions) error {
	result, err := r.useCase.Execute(ctx, param.DeleteInput{Name: opts.name})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Deleted setting %s", result.Name)

	return nil
}
