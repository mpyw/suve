// Package set provides the SSM set command.
package set

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

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
   suve ssm set -s /app/config/api-key "secret123"       Create SecureString
   suve ssm set -t StringList /app/hosts "a.com,b.com"   Create StringList
   suve ssm set -d "DB URL" /app/db-url "postgres://..." With description`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "type",
				Aliases: []string{"t"},
				Value:   "String",
				Usage:   "Parameter type (String, StringList, SecureString)",
			},
			&cli.BoolFlag{
				Name:    "secure",
				Aliases: []string{"s"},
				Usage:   "Shorthand for --type SecureString",
			},
			&cli.StringFlag{
				Name:    "description",
				Aliases: []string{"d"},
				Usage:   "Parameter description",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve ssm set <name> <value>")
	}

	secure := c.Bool("secure")
	paramType := c.String("type")

	// Check for conflicting flags
	if secure && c.IsSet("type") {
		return fmt.Errorf("cannot use --secure with --type; use one or the other")
	}
	if secure {
		paramType = "SecureString"
	}

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: c.App.Writer,
		Stderr: c.App.ErrWriter,
	}
	return r.Run(c.Context, Options{
		Name:        c.Args().Get(0),
		Value:       c.Args().Get(1),
		Type:        paramType,
		Description: c.String("description"),
	})
}

// Run executes the set command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &ssm.PutParameterInput{
		Name:      aws.String(opts.Name),
		Value:     aws.String(opts.Value),
		Type:      types.ParameterType(opts.Type),
		Overwrite: aws.Bool(true),
	}
	if opts.Description != "" {
		input.Description = aws.String(opts.Description)
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
