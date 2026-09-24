package secret

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// CreateRunner executes the create command.
type CreateRunner struct {
	UseCase *secret.CreateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// CreateOptions holds the options for the create command.
type CreateOptions struct {
	Name        string
	Value       string
	Description string
}

// CreateCommand returns the create command.
func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> [<value>]",
		Description: `Create a new secret in AWS Secrets Manager.

Use this command for new secrets only. To update an existing secret,
use 'suve aws secret update' instead.

Secret values are automatically encrypted by Secrets Manager using
the default KMS key or a custom KMS key configured in the account.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

To add tags after creation, use 'suve aws secret tag' command.

EXAMPLES:
   suve aws secret create my-api-key "sk-12345"                    Create simple secret
   suve aws secret create --description "API Key for X" my-key "..." With description
   suve aws secret create my-config '{"host":"db.example.com"}'    Create JSON secret
   printf '%s' "$VALUE" | suve aws secret create my-key --value-stdin  Read value from stdin
   suve aws secret create my-key                                   Type value into $EDITOR`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Description for the secret",
			},
			cliinternal.ValueStdinFlag(),
		},
		Action: createAction,
	}
}

func createAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve aws secret create <name> [<value>]")
	}

	value, proceed, err := cliinternal.ResolveValue(ctx, cliinternal.ValueSource{
		FromStdin: cmd.Bool(cliinternal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     cliinternal.ValueStdin(cmd),
	})
	if err != nil {
		return err
	}

	if !proceed {
		output.Info(cmd.Root().Writer, "Empty value, nothing to create.")

		return nil
	}

	store, err := awsinternal.SecretStore(ctx)
	if err != nil {
		return err
	}

	r := &CreateRunner{
		UseCase: &secret.CreateUseCase{Writer: store},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, CreateOptions{
		Name:        args.Get(0),
		Value:       value,
		Description: cmd.String("description"),
	})
}

// Run executes the create command.
func (r *CreateRunner) Run(ctx context.Context, opts CreateOptions) error {
	result, err := r.UseCase.Execute(ctx, secret.CreateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created secret %s (version: %s)", result.Name, result.Version)

	return nil
}
