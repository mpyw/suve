// Package update provides the Secrets Manager update command.
package update

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the update command.
type Runner struct {
	UseCase *secret.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the update command.
type Options struct {
	Name        string
	Value       string
	Description string
}

// Command returns the update command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a secret value",
		ArgsUsage: "<name> [<value>]",
		Description: `Update the value of an existing secret.

This creates a new version of the secret. The previous version will
have its AWSCURRENT label moved to AWSPREVIOUS.

Use 'suve secret create' to create a new secret.
To manage tags, use 'suve secret tag' and 'suve secret untag' commands.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
  suve secret update my-api-key "new-key-value"         Update with new value
  suve secret update my-config '{"host":"new-db.com"}'  Update JSON secret
  suve secret update --yes my-api-key "new-key-value"   Update without confirmation
  printf '%s' "$VALUE" | suve secret update --yes my-key --value-stdin  Read value from stdin
  suve secret update my-key                             Type value into $EDITOR`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Update secret description",
			},
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
			internal.ValueStdinFlag(),
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve secret update <name> [<value>]")
	}

	name := args.Get(0)
	skipConfirm := cmd.Bool("yes")

	newValue, proceed, err := internal.ResolveValue(ctx, internal.ValueSource{
		FromStdin: cmd.Bool(internal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     internal.Stdin(cmd),
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

	store, err := internal.SecretStore(ctx)
	if err != nil {
		return err
	}

	uc := &secret.UpdateUseCase{Store: store}

	// Fetch current value and show diff before confirming
	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(cmd.Root().ErrWriter, name+" (AWS)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		// Confirm operation
		prompter := &confirm.Prompter{
			Stdin:  internal.Stdin(cmd),
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}
		if identity, _ := infra.GetAWSIdentity(ctx); identity != nil {
			prompter.AccountID = identity.AccountID
			prompter.Region = identity.Region
			prompter.Profile = identity.Profile
		}

		confirmed, err := prompter.ConfirmAction("Update secret", name, false)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	r := &Runner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name:        name,
		Value:       newValue,
		Description: cmd.String("description"),
	})
}

// Run executes the update command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.UpdateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Updated secret %s (version: %s)", result.Name, result.VersionID)

	return nil
}
