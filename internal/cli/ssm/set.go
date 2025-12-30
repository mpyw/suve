package ssm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
)

func setCommand() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Set parameter value",
		ArgsUsage: "<name> <value>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "type",
				Aliases: []string{"t"},
				Value:   "String",
				Usage:   "Parameter type (String, StringList, SecureString)",
			},
			&cli.StringFlag{
				Name:  "description",
				Usage: "Parameter description",
			},
		},
		Action: setAction,
	}
}

func setAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve ssm set <name> <value>")
	}

	name := c.Args().Get(0)
	value := c.Args().Get(1)

	ctx := context.Background()
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Set(ctx, client, c.App.Writer, name, value, c.String("type"), c.String("description"))
}

// Set sets parameter value.
func Set(ctx context.Context, client SetClient, w io.Writer, name, value, paramType, description string) error {
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
	_, _ = fmt.Fprintf(w, "%s %s (version %d)\n", green("Set"), name, result.Version)

	return nil
}
