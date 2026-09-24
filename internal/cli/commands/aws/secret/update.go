package secret

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// UpdateRunner executes the update command.
type UpdateRunner struct {
	UseCase *secret.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// UpdateOptions holds the options for the update command.
type UpdateOptions struct {
	Name        string
	Value       string
	Description string
}

// UpdateCommand returns the update command.
func UpdateCommand() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a secret value",
		ArgsUsage: "<name> [<value>]",
		Description: `Update the value of an existing secret.

This creates a new version of the secret. The previous version will
have its AWSCURRENT label moved to AWSPREVIOUS.

Use 'suve aws secret create' to create a new secret.
To manage tags, use 'suve aws secret tag' and 'suve aws secret untag' commands.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
  suve aws secret update my-api-key "new-key-value"         Update with new value
  suve aws secret update my-config '{"host":"new-db.com"}'  Update JSON secret
  suve aws secret update --yes my-api-key "new-key-value"   Update without confirmation
  printf '%s' "$VALUE" | suve aws secret update --yes my-key --value-stdin  Read value from stdin
  suve aws secret update my-key                             Type value into $EDITOR`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Update secret description",
			},
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
		return errors.New("usage: suve aws secret update <name> [<value>]")
	}

	name := args.Get(0)
	skipConfirm := cmd.Bool("yes")

	newValue, proceed, err := cliinternal.ResolveValue(ctx, cliinternal.ValueSource{
		FromStdin: cmd.Bool(cliinternal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     cliinternal.ValueStdin(cmd),
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

	store, err := awsinternal.SecretStore(ctx)
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
			Stdin:  cliinternal.ValueStdin(cmd),
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}
		if identity, _ := aws.LoadIdentity(ctx); identity != nil {
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

	r := &UpdateRunner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, UpdateOptions{
		Name:        name,
		Value:       newValue,
		Description: cmd.String("description"),
	})
}

// Run executes the update command.
func (r *UpdateRunner) Run(ctx context.Context, opts UpdateOptions) error {
	result, err := r.UseCase.Execute(ctx, secret.UpdateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Updated secret %s (version: %s)", result.Name, result.Version)

	return nil
}
