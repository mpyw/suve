// Package update provides the SSM Parameter Store update command.
package update

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the update command.
type Runner struct {
	UseCase *param.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the update command.
type Options struct {
	Name        string
	Value       string
	Type        string
	Description string
}

// Command returns the update command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a parameter value",
		ArgsUsage: "<name> <value>",
		Description: `Update the value of an existing parameter.

This creates a new version of the parameter in AWS Systems Manager Parameter Store.

Use 'suve param create' to create a new parameter.
To manage tags, use 'suve param tag' and 'suve param untag' commands.

PARAMETER TYPES:
   String        Plain text value (default)
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

EXAMPLES:
   suve param update /app/config/db-url "postgres://..."       Update parameter
   suve param update --secure /app/config/api-key "secret123"  Update as SecureString
   suve param update --yes /app/config/db-url "postgres://..." Update without confirmation`,
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
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and value
		return fmt.Errorf("usage: suve param update <name> <value>")
	}

	secure := cmd.Bool("secure")
	paramType := cmd.String("type")

	// Check for conflicting flags
	if secure && cmd.IsSet("type") {
		return fmt.Errorf("cannot use --secure with --type; use one or the other")
	}

	if secure {
		paramType = "SecureString"
	}

	name := cmd.Args().Get(0)
	skipConfirm := cmd.Bool("yes")

	adapter, err := awsparam.NewAdapter(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	uc := &param.UpdateUseCase{Client: adapter}
	newValue := cmd.Args().Get(1)

	// Fetch current value and show diff before confirming
	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(name+" (AWS)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		// Confirm operation
		prompter := &confirm.Prompter{
			Stdin:  os.Stdin,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}
		if identity, _ := infra.GetAWSIdentity(ctx); identity != nil {
			prompter.AccountID = identity.AccountID
			prompter.Region = identity.Region
			prompter.Profile = identity.Profile
		}

		confirmed, err := prompter.ConfirmAction("Update parameter", name, false)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	r := &Runner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name:        name,
		Value:       newValue,
		Type:        paramType,
		Description: cmd.String("description"),
	})
}

// Run executes the update command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.UpdateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Type:        opts.Type,
		Description: opts.Description,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Updated parameter %s (version: %d)", result.Name, result.Version)

	return nil
}
