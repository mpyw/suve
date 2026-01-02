// Package show provides the Secrets Manager show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// Client is the interface for the show command.
type Client interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIdsAPI
}

// Runner executes the show command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the show command.
type Options struct {
	Spec       *secretversion.Spec
	ParseJSON bool
	NoPager    bool
	Raw        bool
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION | :LABEL][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

Use --raw to output only the value without metadata (for piping/scripting).

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve secret show my-secret                 Show current version
  suve secret show my-secret~                Show previous version
  suve secret show my-secret:AWSPREVIOUS     Show AWSPREVIOUS label
  suve secret show my-secret:AWSPREVIOUS~1   Show 1 before AWSPREVIOUS
  suve secret show -j my-secret              Pretty print JSON value
  suve secret show --raw my-secret           Output raw value (for piping)
  API_KEY=$(suve secret show --raw my-secret)  Use in shell variable`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values (keys are always sorted alphabetically)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.BoolFlag{
				Name:  "raw",
				Usage: "Output raw value only without metadata (for piping)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve secret show <name>")
	}

	spec, err := secretversion.Parse(cmd.Args().First())
	if err != nil {
		return err
	}

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec:       spec,
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:    cmd.Bool("no-pager"),
		Raw:        cmd.Bool("raw"),
	}

	// Raw mode disables pager
	noPager := opts.NoPager || opts.Raw

	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		r := &Runner{
			Client: client,
			Stdout: w,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}

// Run executes the show command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	secret, err := secretversion.GetSecretWithVersion(ctx, r.Client, opts.Spec)
	if err != nil {
		return err
	}

	value := lo.FromPtr(secret.SecretString)

	// Format as JSON if enabled
	if opts.ParseJSON {
		value = jsonutil.TryFormatOrWarn(value, r.Stderr, "")
	}

	// Raw mode: output value only without trailing newline
	if opts.Raw {
		_, _ = fmt.Fprint(r.Stdout, value)
		return nil
	}

	// Normal mode: show metadata + value
	out := output.New(r.Stdout)
	out.Field("Name", lo.FromPtr(secret.Name))
	out.Field("ARN", lo.FromPtr(secret.ARN))
	if secret.VersionId != nil {
		out.Field("VersionId", lo.FromPtr(secret.VersionId))
	}
	if len(secret.VersionStages) > 0 {
		out.Field("Stages", fmt.Sprintf("%v", secret.VersionStages))
	}
	if secret.CreatedDate != nil {
		out.Field("Created", secret.CreatedDate.Format(time.RFC3339))
	}
	out.Separator()
	out.Value(value)

	return nil
}
