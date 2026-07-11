package param

import (
	"context"
	"errors"
	"io"

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
		ArgsUsage: "<key> [<value>]",
		Description: `Update the value of an existing setting.

App Configuration is unversioned: the value is replaced in place.
Use 'suve azure param create' to create a new setting.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
  suve azure param update app/timeout "60"        Replace the value
  suve azure param update --yes app/timeout "60"  Update without confirmation
  printf '%s' "$V" | suve azure param update --yes app/key --value-stdin  Read value from stdin
  suve azure param update app/key                 Type value into $EDITOR`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
			cliinternal.ValueStdinFlag(),
		},
		Action: updateAction,
	}
}

func updateAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve azure param update <key> [<value>]")
	}

	name := args.Get(0)
	skipConfirm := cmd.Bool("yes")

	newValue, proceed, err := cliinternal.ResolveValue(ctx, cliinternal.ValueSource{
		FromStdin: cmd.Bool(cliinternal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the key, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     cliinternal.Stdin(cmd),
		// Without --yes we prompt for confirmation on the same stdin below;
		// reading the value from stdin would leave nothing for that prompt.
		ConfirmRequired: !skipConfirm,
	})
	if err != nil {
		return err
	}

	if !proceed {
		output.Info(cmd.Root().Writer, "Empty value, nothing to update.")

		return nil
	}

	store, err := cliinternal.AzureAppConfigStore(ctx)
	if err != nil {
		return err
	}

	uc := &azure.UpdateUseCase{Store: store}

	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(cmd.Root().ErrWriter, name+" (current)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		prompter := &confirm.Prompter{
			Stdin:  cliinternal.Stdin(cmd),
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
