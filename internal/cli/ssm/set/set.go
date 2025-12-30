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

	name := c.Args().Get(0)
	value := c.Args().Get(1)
	secure := c.Bool("secure")
	paramType := c.String("type")
	description := c.String("description")

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

	return Run(c.Context, client, c.App.Writer, name, value, paramType, description)
}

// Run executes the set command.
func Run(ctx context.Context, client Client, w io.Writer, name, value, paramType, description string) error {
	input := &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      types.ParameterType(paramType),
		Overwrite: aws.Bool(true),
	}
	if description != "" {
		input.Description = aws.String(description)
	}

	result, err := client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(w, "%s Set parameter %s (version: %d)\n",
		green("✓"),
		name,
		result.Version,
	)

	return nil
}
