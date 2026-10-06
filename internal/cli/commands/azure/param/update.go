package param

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/usecase/param"
)

// updateRunner executes the update command.
type updateRunner struct {
	useCase *param.UpdateUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// updateOptions holds the options for the update command.
type updateOptions struct {
	name  string
	value string
}

// updateCommand returns the Azure App Configuration update command.
//
//declscope:shared // command.go registers it
func updateCommand() *cli.Command {
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
			valueinput.ValueStdinFlag(),
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

	newValue, proceed, err := valueinput.ResolveValue(ctx, valueinput.ValueSource{
		FromStdin: cmd.Bool(valueinput.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the key, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     valueinput.ValueStdin(cmd),
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

	store, err := azureinternal.AppConfigStore(ctx)
	if err != nil {
		return err
	}

	uc := &param.UpdateUseCase{Store: store, ItemNoun: itemNoun()}

	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(cmd.Root().ErrWriter, name+" (current)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		prompter := &confirm.Prompter{
			Stdin:  valueinput.ValueStdin(cmd),
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
			Target: azureinternal.AppConfigConfirmTarget(ctx),
		}

		confirmed, cerr := prompter.ConfirmAction("Update setting", name, false)
		if cerr != nil {
			return cerr
		}

		if !confirmed {
			return nil
		}
	}

	r := &updateRunner{
		useCase: uc,
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, updateOptions{name: name, value: newValue})
}

// run executes the update command.
func (r *updateRunner) run(ctx context.Context, opts updateOptions) error {
	result, err := r.useCase.Execute(ctx, param.UpdateInput{
		Name:  opts.name,
		Value: opts.value,
		Type:  domain.ValueTypePlaintext,
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Updated setting %s", result.Name)

	return nil
}
