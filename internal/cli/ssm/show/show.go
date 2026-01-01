// Package show provides the SSM show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the show command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// Runner executes the show command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the show command.
type Options struct {
	Spec       *ssmversion.Spec
	Decrypt    bool
	JSONFormat bool
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show parameter value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a parameter's value along with its metadata (name, version, type, modification date).

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago (e.g., ~1, ~2); ~ alone means ~1

EXAMPLES:
  suve ssm show /app/config/db-url              Show latest version
  suve ssm show /app/config/db-url#3            Show version 3
  suve ssm show /app/config/db-url~             Show previous version
  suve ssm show -j /app/config/db-url           Pretty print JSON value
  suve ssm show --decrypt=false /app/secret     Show without decryption`,
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

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm show <name>")
	}

	spec, err := ssmversion.Parse(cmd.Args().First())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSSMClient(ctx)
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
		Decrypt:    cmd.Bool("decrypt"),
		JSONFormat: cmd.Bool("json"),
	})
}

// Run executes the show command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	param, err := ssmversion.GetParameterWithVersion(ctx, r.Client, opts.Spec, opts.Decrypt)
	if err != nil {
		return err
	}

	out := output.New(r.Stdout)
	out.Field("Name", lo.FromPtr(param.Name))
	out.Field("Version", fmt.Sprintf("%d", param.Version))
	out.Field("Type", string(param.Type))
	if param.LastModifiedDate != nil {
		out.Field("Modified", param.LastModifiedDate.Format(time.RFC3339))
	}
	out.Separator()

	value := lo.FromPtr(param.Value)

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
	out.Value(value)

	return nil
}
