package param

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// UpdateRunner executes the update command.
type UpdateRunner struct {
	UseCase *azure.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// UpdateOptions holds the options for the update command.
type UpdateOptions struct {
	Name  string
	Value string
}

// UpdateCommand returns the Azure App Configuration update command.
func UpdateCommand() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a setting value",
		ArgsUsage: "<key> <value>",
		Description: `Update the value of an existing setting.

App Configuration is unversioned: the value is replaced in place.
Use 'suve azure param create' to create a new setting.

EXAMPLES:
  suve azure param update app/timeout "60"        Replace the value
  suve azure param update --yes app/timeout "60"  Update without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: updateAction,
	}
}

func updateAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: key and value
		return fmt.Errorf("usage: suve azure param update <key> <value>")
	}

	name := cmd.Args().Get(0)
	newValue := cmd.Args().Get(1)
	skipConfirm := cmd.Bool("yes")

	store, err := cliinternal.AzureAppConfigStore(ctx)
	if err != nil {
		return err
	}

	uc := &azure.UpdateUseCase{Store: store}

	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(name+" (current)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		prompter := &confirm.Prompter{
			Stdin:  os.Stdin,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		confirmed, cerr := prompter.ConfirmAction("Update setting", name, false)
		if cerr != nil {
			return cerr
		}

		if !confirmed {
			return nil
		}
	}

	r := &UpdateRunner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, UpdateOptions{Name: name, Value: newValue})
}

// Run executes the update command.
func (r *UpdateRunner) Run(ctx context.Context, opts UpdateOptions) error {
	result, err := r.UseCase.Execute(ctx, azure.UpdateInput{
		Name:      opts.Name,
		Value:     opts.Value,
		ValueType: domain.ValueTypePlaintext,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Updated setting %s", result.Name)

	return nil
}
