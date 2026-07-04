package gcloud

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/gcp"
)

// CreateRunner executes the create command.
type CreateRunner struct {
	UseCase *gcp.CreateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// CreateOptions holds the options for the create command.
type CreateOptions struct {
	Name  string
	Value string
}

// CreateCommand returns the Google Cloud Secret Manager create command.
func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> <value>",
		Description: `Create a new secret in Google Cloud Secret Manager.

Use this command for new secrets only. To add a new version to an existing
secret, use 'suve gcloud secret update' instead.

The secret is created with automatic replication, and the given value becomes
its first version. To add labels after creation, use 'suve gcloud secret tag'.

EXAMPLES:
   suve gcloud secret create my-api-key "sk-12345"             Create simple secret
   suve gcloud secret create my-config '{"host":"db"}'         Create JSON secret`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and value
				return fmt.Errorf("usage: suve gcloud secret create <name> <value>")
			}

			store, err := cliinternal.GCPSecretStore(ctx)
			if err != nil {
				return err
			}

			r := &CreateRunner{
				UseCase: &gcp.CreateUseCase{Writer: store},
				Stdout:  cmd.Root().Writer,
				Stderr:  cmd.Root().ErrWriter,
			}

			return r.Run(ctx, CreateOptions{Name: cmd.Args().Get(0), Value: cmd.Args().Get(1)})
		},
	}
}

// Run executes the create command.
func (r *CreateRunner) Run(ctx context.Context, opts CreateOptions) error {
	result, err := r.UseCase.Execute(ctx, gcp.CreateInput{Name: opts.Name, Value: opts.Value})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created secret %s (version: %s)", result.Name, result.Version)

	return nil
}
