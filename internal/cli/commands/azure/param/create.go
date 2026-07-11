package param

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// CreateRunner executes the create command.
type CreateRunner struct {
	UseCase *azure.CreateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// CreateOptions holds the options for the create command.
type CreateOptions struct {
	Name  string
	Value string
}

// CreateCommand returns the Azure App Configuration create command.
func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new setting",
		ArgsUsage: "<key> [<value>]",
		Description: `Create a new setting (key-value) in Azure App Configuration.

Use this command for new keys only. To change the value of an existing key, use
'suve azure param update' instead.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
   suve azure param create app/timeout "30"                  Create simple setting
   suve azure param create app/config '{"host":"db"}'        Create JSON setting
   printf '%s' "$V" | suve azure param create app/key --value-stdin  Read value from stdin
   suve azure param create app/key                           Type value into $EDITOR`,
		Flags: []cli.Flag{
			cliinternal.ValueStdinFlag(),
		},
		Action: createAction,
	}
}

func createAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve azure param create <key> [<value>]")
	}

	value, proceed, err := cliinternal.ResolveValue(ctx, cliinternal.ValueSource{
		FromStdin: cmd.Bool(cliinternal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the key, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     cliinternal.Stdin(cmd),
	})
	if err != nil {
		return err
	}

	if !proceed {
		output.Info(cmd.Root().Writer, "Empty value, nothing to create.")

		return nil
	}

	store, err := cliinternal.AzureAppConfigStore(ctx)
	if err != nil {
		return err
	}

	r := &CreateRunner{
		UseCase: &azure.CreateUseCase{Writer: store},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, CreateOptions{Name: args.Get(0), Value: value})
}

// Run executes the create command.
func (r *CreateRunner) Run(ctx context.Context, opts CreateOptions) error {
	result, err := r.UseCase.Execute(ctx, azure.CreateInput{
		Name:      opts.Name,
		Value:     opts.Value,
		ValueType: domain.ValueTypePlaintext,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created setting %s", result.Name)

	return nil
}
