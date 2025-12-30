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

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/ssmapi"
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
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "type",
				Aliases: []string{"t"},
				Value:   "SecureString",
				Usage:   "Parameter type (String, StringList, SecureString)",
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
	paramType := c.String("type")
	description := c.String("description")

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
