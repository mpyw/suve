package secret

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

// CreateCommand returns the Azure Key Vault create command.
func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> <value>",
		Description: `Create a new secret in Azure Key Vault.

Use this command for new secrets only. To add a new version to an existing
secret, use 'suve azure secret update' instead.

The given value becomes the secret's first version. To add tags after creation,
use 'suve azure secret tag'.

EXAMPLES:
   suve azure secret create my-api-key "sk-12345"             Create simple secret
   suve azure secret create my-config '{"host":"db"}'         Create JSON secret`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and value
				return fmt.Errorf("usage: suve azure secret create <name> <value>")
			}

			store, err := cliinternal.AzureKeyVaultStore(ctx)
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
		ValueType: domain.ValueTypeSecret,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created secret %s (version: %s)", result.Name, result.Version)

	return nil
}
