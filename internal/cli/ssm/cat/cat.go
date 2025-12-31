// Package cat provides the SSM cat command.
package cat

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the cat command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// Runner executes the cat command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the cat command.
type Options struct {
	Spec       *ssmversion.Spec
	Decrypt    bool
	JSONFormat bool
}

// Command returns the cat command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "cat",
		Usage:     "Output raw parameter value (for piping)",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Output the raw parameter value without any formatting.
Does not append a trailing newline. Designed for scripts and piping.

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago (e.g., ~1, ~2); ~ alone means ~1

EXAMPLES:
  suve ssm cat /app/config/db-url            Output latest value
  suve ssm cat /app/config/db-url#3          Output version 3
  suve ssm cat /app/config/db-url~           Output previous version
  suve ssm cat -j /app/config/db-url         Pretty print JSON value
  DB_URL=$(suve ssm cat /app/config/db-url)  Use in shell variable`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "decrypt",
				Aliases: []string{"d"},
				Value:   true,
				Usage:   "Decrypt SecureString values (use --decrypt=false to disable)",
			},
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values (keys are always sorted alphabetically)",
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

	r := &Runner{
		Client: client,
		Stdout: c.App.Writer,
		Stderr: c.App.ErrWriter,
	}
	return r.Run(c.Context, Options{
		Spec:       spec,
		Decrypt:    c.Bool("decrypt"),
		JSONFormat: c.Bool("json"),
	})
}

// Run executes the cat command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	param, err := ssmversion.GetParameterWithVersion(ctx, r.Client, opts.Spec, opts.Decrypt)
	if err != nil {
		return err
	}

	value := aws.ToString(param.Value)

	// Warn if --json is used in cases where it's not meaningful
	if opts.JSONFormat {
		switch {
		case param.Type == types.ParameterTypeStringList:
			output.Warning(r.Stderr, "--json has no effect on StringList type (comma-separated values)")
		case param.Type == types.ParameterTypeSecureString && !opts.Decrypt:
			output.Warning(r.Stderr, "--json has no effect on encrypted SecureString (use --decrypt to enable)")
		case !jsonutil.IsJSON(value):
			output.Warning(r.Stderr, "--json has no effect: value is not valid JSON")
		default:
			value = jsonutil.Format(value)
		}
	}

	_, _ = fmt.Fprint(r.Stdout, value)
	return nil
}
