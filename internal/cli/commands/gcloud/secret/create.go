package secret

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	gcloudinternal "github.com/mpyw/suve/internal/cli/commands/gcloud/internal"
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
	name        string
	value       string
	description string
}

// createCommand returns the Google Cloud Secret Manager create command.
//
//declscope:shared // command.go registers it
func createCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> [<value>]",
		Description: `Create a new secret in Google Cloud Secret Manager.

Use this command for new secrets only. To add a new version to an existing
secret, use 'suve gcloud secret update' instead.

The secret is created with automatic replication, and the given value becomes
its first version. To add labels after creation, use 'suve gcloud secret tag'.

A --description is stored as the secret's "description" annotation (Google Cloud
secrets have no native description field; the annotation axis is distinct from
the labels that back tags).

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
   suve gcloud secret create my-api-key "sk-12345"             Create simple secret
   suve gcloud secret create my-config '{"host":"db"}'         Create JSON secret
   printf '%s' "$V" | suve gcloud secret create my-key --value-stdin  Read value from stdin
   suve gcloud secret create my-key                            Type value into $EDITOR`,
		Flags: []cli.Flag{
			valueinput.ValueStdinFlag(),
			&cli.StringFlag{
				Name:  "description",
				Usage: "Description for the secret (stored as the \"description\" annotation)",
			},
		},
		Action: createAction,
	}
}

func createAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve gcloud secret create <name> [<value>]")
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

	store, err := gcloudinternal.SecretStore(ctx)
	if err != nil {
		return err
	}

	r := &createRunner{
		useCase: &secret.CreateUseCase{Writer: store},
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, createOptions{name: args.Get(0), value: value, description: cmd.String("description")})
}

// run executes the create command.
func (r *createRunner) run(ctx context.Context, opts createOptions) error {
	result, err := r.useCase.Execute(ctx, secret.CreateInput{Name: opts.name, Value: opts.value, Description: opts.description})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Created secret %s (version: %s)", result.Name, result.Version)

	return nil
}
