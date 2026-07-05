package param

import (
	"context"
	"fmt"
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
		ArgsUsage: "<key> <value>",
		Description: `Create a new setting (key-value) in Azure App Configuration.

Use this command for new keys only. To change the value of an existing key, use
'suve azure param update' instead.

EXAMPLES:
   suve azure param create app/timeout "30"                  Create simple setting
   suve azure param create app/config '{"host":"db"}'        Create JSON setting`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: key and value
				return fmt.Errorf("usage: suve azure param create <key> <value>")
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

			return r.Run(ctx, CreateOptions{Name: cmd.Args().Get(0), Value: cmd.Args().Get(1)})
		},
	}
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
