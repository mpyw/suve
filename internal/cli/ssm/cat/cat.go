// Package cat provides the SSM cat command.
package cat

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the cat command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// Command returns the cat command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "cat",
		Usage:     "Output raw parameter value (for piping)",
		ArgsUsage: "<name[#version][shifts]>",
		Description: `Output the raw parameter value without any formatting.
Does not append a trailing newline. Designed for scripts and piping.

VERSION SPECIFIERS:
  #N   Specific version (e.g., #3)
  ~N   N versions ago (e.g., ~1, ~2); ~ alone means ~1

EXAMPLES:
  suve ssm cat /app/config/db-url            Output latest value
  suve ssm cat /app/config/db-url#3          Output version 3
  suve ssm cat /app/config/db-url~           Output previous version
  DB_URL=$(suve ssm cat /app/config/db-url)  Use in shell variable`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "decrypt",
				Aliases: []string{"d"},
				Value:   true,
				Usage:   "Decrypt SecureString values (use --decrypt=false to disable)",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	spec, err := ssmversion.Parse(c.Args().First())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, spec, c.Bool("decrypt"))
}

// Run executes the cat command.
func Run(ctx context.Context, client Client, w io.Writer, spec *ssmversion.Spec, decrypt bool) error {
	param, err := ssmversion.GetParameterWithVersion(ctx, client, spec, decrypt)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(w, aws.ToString(param.Value))
	return nil
}
