// Package create provides the Secrets Manager create command.
package create

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the create command.
type Runner struct {
	UseCase *secret.CreateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the create command.
type Options struct {
	Name        string
	Value       string
	Description string
}

// Command returns the create command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> <value>",
		Description: `Create a new secret in AWS Secrets Manager.

Use this command for new secrets only. To update an existing secret,
use 'suve secret update' instead.

Secret values are automatically encrypted by Secrets Manager using
the default KMS key or a custom KMS key configured in the account.

To add tags after creation, use 'suve secret tag' command.

EXAMPLES:
   suve secret create my-api-key "sk-12345"                    Create simple secret
   suve secret create --description "API Key for X" my-key "..." With description
   suve secret create my-config '{"host":"db.example.com"}'    Create JSON secret`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Description for the secret",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve secret create <name> <value>")
	}

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		UseCase: &secret.CreateUseCase{Client: client},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name:        cmd.Args().Get(0),
		Value:       cmd.Args().Get(1),
		Description: cmd.String("description"),
	})
}

// Run executes the create command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.CreateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created secret %s (version: %s)", result.Name, result.VersionID)

	return nil
}
