// Package cat provides the SM cat command.
package cat

import (
	"context"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the cat command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the cat command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the cat command.
type Options struct {
	Spec       *smversion.Spec
	JSONFormat bool
}

// Command returns the cat command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "cat",
		Usage:     "Output raw secret value (for piping)",
		ArgsUsage: "<name[#VERSION | :LABEL][~SHIFT]*>",
		Description: `Output the raw secret value without any formatting.
Does not append a trailing newline. Designed for scripts and piping.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm cat my-secret              Output current value
  suve sm cat my-secret~             Output previous version
  suve sm cat my-secret:AWSPREVIOUS  Output AWSPREVIOUS label
  suve sm cat -j my-secret           Pretty print JSON value
  API_KEY=$(suve sm cat my-api-key)  Use in shell variable`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values (keys are always sorted alphabetically)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm cat <name>")
	}

	spec, err := smversion.Parse(cmd.Args().First())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Spec:       spec,
		JSONFormat: cmd.Bool("json"),
	})
}

// Run executes the cat command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	secret, err := smversion.GetSecretWithVersion(ctx, r.Client, opts.Spec)
	if err != nil {
		return err
	}

	value := lo.FromPtr(secret.SecretString)

	// Format as JSON if enabled
	if opts.JSONFormat {
		if formatted, ok := jsonutil.TryFormat(value); ok {
			value = formatted
		} else {
			output.Warning(r.Stderr, "--json has no effect: value is not valid JSON")
		}
	}

	_, _ = fmt.Fprint(r.Stdout, value)
	return nil
}
