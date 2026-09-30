package secret

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// createRunner executes the create command.
type createRunner struct {
	useCase *secret.CreateUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// createOptions holds the options for the create command.
type createOptions struct {
	name  string
	value string
}

// createCommand returns the Azure Key Vault create command.
//
//declscope:package // command.go registers it
func createCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> [<value>]",
		Description: `Create a new secret in Azure Key Vault.

Use this command for new secrets only. To add a new version to an existing
secret, use 'suve azure secret update' instead.

The given value becomes the secret's first version. To add tags after creation,
use 'suve azure secret tag'.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
   suve azure secret create my-api-key "sk-12345"             Create simple secret
   suve azure secret create my-config '{"host":"db"}'         Create JSON secret
   printf '%s' "$V" | suve azure secret create my-key --value-stdin  Read value from stdin
   suve azure secret create my-key                            Type value into $EDITOR`,
		Flags: []cli.Flag{
			valueinput.ValueStdinFlag(),
		},
		Action: createAction,
	}
}

func createAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve azure secret create <name> [<value>]")
	}

	value, proceed, err := valueinput.ResolveValue(ctx, valueinput.ValueSource{
		FromStdin: cmd.Bool(valueinput.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     valueinput.ValueStdin(cmd),
	})
	if err != nil {
		return err
	}

	if !proceed {
		output.Info(cmd.Root().Writer, "Empty value, nothing to create.")

		return nil
	}

	store, err := azureinternal.KeyVaultStore(ctx)
	if err != nil {
		return err
	}

	r := &createRunner{
		useCase: &secret.CreateUseCase{Writer: store},
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, createOptions{name: args.Get(0), value: value})
}

// run executes the create command.
func (r *createRunner) run(ctx context.Context, opts createOptions) error {
	result, err := r.useCase.Execute(ctx, secret.CreateInput{Name: opts.name, Value: opts.value})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Created secret %s (version: %s)", result.Name, result.Version)

	return nil
}
