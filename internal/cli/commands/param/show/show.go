// Package show provides the SSM Parameter Store show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// Client is the interface for the show command.
type Client interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
}

// Runner executes the show command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the show command.
type Options struct {
	Spec       *paramversion.Spec
	Decrypt    bool
	ParseJSON bool
	NoPager    bool
	Raw        bool
}

// Command returns the show command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show parameter value with metadata",
		ArgsUsage: "<name[#VERSION][~SHIFT]*>",
		Description: `Display a parameter's value along with its metadata (name, version, type, modification date).

Use --raw to output only the value without metadata (for piping/scripting).

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago (e.g., ~1, ~2); ~ alone means ~1

EXAMPLES:
  suve param show /app/config/db-url              Show latest version
  suve param show /app/config/db-url#3            Show version 3
  suve param show /app/config/db-url~             Show previous version
  suve param show -j /app/config/db-url           Pretty print JSON value
  suve param show --decrypt=false /app/secret     Show without decryption
  suve param show --raw /app/config/db-url        Output raw value (for piping)
  DB_URL=$(suve param show --raw /app/config/db-url)  Use in shell variable`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "decrypt",
				Value: true,
				Usage: "Decrypt SecureString values (use --decrypt=false to disable)",
			},
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
		return fmt.Errorf("usage: suve param show <name>")
	}

	spec, err := paramversion.Parse(cmd.Args().First())
	if err != nil {
		return err
	}

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec:       spec,
		Decrypt:    cmd.Bool("decrypt"),
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
	param, err := paramversion.GetParameterWithVersion(ctx, r.Client, opts.Spec, opts.Decrypt)
	if err != nil {
		return err
	}

	value := lo.FromPtr(param.Value)

	// Warn if --parse-json is used in cases where it's not meaningful
	if opts.ParseJSON {
		switch {
		case param.Type == paramapi.ParameterTypeStringList:
			output.Warning(r.Stderr, "--parse-json has no effect on StringList type (comma-separated values)")
		case param.Type == paramapi.ParameterTypeSecureString && !opts.Decrypt:
			output.Warning(r.Stderr, "--parse-json has no effect on encrypted SecureString (use --decrypt to enable)")
		default:
			value = jsonutil.TryFormatOrWarn(value, r.Stderr, "")
		}
	}

	// Raw mode: output value only without trailing newline
	if opts.Raw {
		_, _ = fmt.Fprint(r.Stdout, value)
		return nil
	}

	// Normal mode: show metadata + value
	out := output.New(r.Stdout)
	out.Field("Name", lo.FromPtr(param.Name))
	out.Field("Version", fmt.Sprintf("%d", param.Version))
	out.Field("Type", string(param.Type))
	if param.LastModifiedDate != nil {
		out.Field("Modified", param.LastModifiedDate.Format(time.RFC3339))
	}
	out.Separator()
	out.Value(value)

	return nil
}
