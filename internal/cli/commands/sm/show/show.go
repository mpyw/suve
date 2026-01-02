// Package show provides the SM show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the show command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the show command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the show command.
type Options struct {
	Spec       *smversion.Spec
	JSONFormat bool
	NoPager    bool
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[#VERSION | :LABEL][~SHIFT]*>",
		Description: `Display a secret's value along with its metadata.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS, or custom)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm show my-secret                 Show current version
  suve sm show my-secret~                Show previous version
  suve sm show my-secret:AWSPREVIOUS     Show AWSPREVIOUS label
  suve sm show my-secret:AWSPREVIOUS~1   Show 1 before AWSPREVIOUS
  suve sm show -j my-secret              Pretty print JSON value`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values (keys are always sorted alphabetically)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm show <name>")
	}

	spec, err := smversion.Parse(cmd.Args().First())
	if err != nil {
		return err
	}

	client, err := infra.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec:       spec,
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
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
	secret, err := smversion.GetSecretWithVersion(ctx, r.Client, opts.Spec)
	if err != nil {
		return err
	}

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

	value := lo.FromPtr(secret.SecretString)

	// Format as JSON if enabled
	if opts.JSONFormat {
		value = jsonutil.TryFormatOrWarn(value, r.Stderr, "")
	}
	out.Value(value)

	return nil
}
