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

func lsCommand() *cli.Command {
	return &cli.Command{
		Name:      "ls",
		Usage:     "List parameters",
		ArgsUsage: "[path-prefix]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "recursive",
				Aliases: []string{"r"},
				Usage:   "List parameters recursively",
			},
			&cli.IntFlag{
				Name:    "max",
				Aliases: []string{"m"},
				Value:   50,
				Usage:   "Maximum number of parameters to list",
			},
		},
		Action: lsAction,
	}
}

func lsAction(c *cli.Context) error {
	prefix := "/"
	if c.NArg() > 0 {
		prefix = c.Args().First()
	}

	ctx := context.Background()
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Ls(ctx, client, c.App.Writer, prefix, c.Bool("recursive"), int32(c.Int("max")))
}

// Ls lists parameters.
func Ls(ctx context.Context, client LsClient, w io.Writer, prefix string, recursive bool, maxResults int32) error {
	option := "OneLevel"
	if recursive {
		option = "Recursive"
	}

	input := &ssm.DescribeParametersInput{
		MaxResults: aws.Int32(maxResults),
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    aws.String("Path"),
				Option: aws.String(option),
				Values: []string{prefix},
			},
		},
	}

	result, err := client.DescribeParameters(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to describe parameters: %w", err)
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	for _, param := range result.Parameters {
		name := aws.ToString(param.Name)
		typeStr := string(param.Type)
		modified := ""
		if param.LastModifiedDate != nil {
			modified = param.LastModifiedDate.Format("2006-01-02 15:04")
		}
		_, _ = fmt.Fprintf(w, "%s  %s  %s\n", cyan(typeStr), modified, name)
	}

	return nil
}
