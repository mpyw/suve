// Package create provides the SSM Parameter Store create command.
package create

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the create command.
type Runner struct {
	UseCase *param.CreateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the create command.
type Options struct {
	Name        string
	Value       string
	Type        string
	Description string
}

// Command returns the create command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new parameter",
		ArgsUsage: "<name> <value>",
		Description: `Create a new parameter in AWS Systems Manager Parameter Store.

Use this command for new parameters only. To update an existing parameter,
use 'suve param update' instead.

PARAMETER TYPES:
   String        Plain text value (default)
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

To add tags after creation, use 'suve param tag' command.

EXAMPLES:
   suve param create /app/config/db-url "postgres://..."       Create String parameter
   suve param create --secure /app/config/api-key "secret123"  Create SecureString
   suve param create --type StringList /app/hosts "a.com,b.com" Create StringList
   suve param create --description "DB URL" /app/db-url "..."  With description`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "type",
				Value: "String",
				Usage: "Parameter type (String, StringList, SecureString)",
			},
			&cli.BoolFlag{
				Name:  "secure",
				Usage: "Shorthand for --type SecureString",
			},
			&cli.StringFlag{
				Name:  "description",
				Usage: "Parameter description",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and value
		return errors.New("usage: suve param create <name> <value>")
	}

	secure := cmd.Bool("secure")
	paramType := cmd.String("type")

	// Check for conflicting flags
	if secure && cmd.IsSet("type") {
		return errors.New("cannot use --secure with --type; use one or the other")
	}

	if secure {
		paramType = "SecureString"
	}

	adapter, err := awsparam.NewAdapter(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		UseCase: &param.CreateUseCase{Client: adapter},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name:        cmd.Args().Get(0),
		Value:       cmd.Args().Get(1),
		Type:        paramType,
		Description: cmd.String("description"),
	})
}

// Run executes the create command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.CreateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Type:        opts.Type,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created parameter %s (version: %d)", result.Name, result.Version)

	return nil
}
