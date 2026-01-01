// Package set provides the SSM set command.
package set

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the set command.
type Client interface {
	ssmapi.PutParameterAPI
}

// Runner executes the set command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the set command.
type Options struct {
	Name        string
	Value       string
	Type        string
	Description string
}

// Command returns the set command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Set parameter value",
		ArgsUsage: "<name> <value>",
		Description: `Create a new parameter or update an existing one.

PARAMETER TYPES:
   String        Plain text value (default)
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

EXAMPLES:
   suve ssm set /app/config/db-url "postgres://..."       Create String parameter
   suve ssm set --secure /app/config/api-key "secret123"        Create SecureString
   suve ssm set --type StringList /app/hosts "a.com,b.com"      Create StringList
   suve ssm set --description "DB URL" /app/db-url "postgres://..." With description`,
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
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve ssm set <name> <value>")
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

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name:        cmd.Args().Get(0),
		Value:       cmd.Args().Get(1),
		Type:        paramType,
		Description: cmd.String("description"),
	})
}

// Run executes the set command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &ssm.PutParameterInput{
		Name:      lo.ToPtr(opts.Name),
		Value:     lo.ToPtr(opts.Value),
		Type:      types.ParameterType(opts.Type),
		Overwrite: lo.ToPtr(true),
	}
	if opts.Description != "" {
		input.Description = lo.ToPtr(opts.Description)
	}

	result, err := r.Client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Set parameter %s (version: %d)\n",
		green("✓"),
		opts.Name,
		result.Version,
	)

	return nil
}
