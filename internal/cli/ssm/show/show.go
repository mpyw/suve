// Package show provides the SSM show command.
package show

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/urfave/cli/v2"

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

	return Run(c.Context, client, c.App.Writer, spec, c.Bool("decrypt"), c.Bool("json"))
}

// Run executes the show command.
func Run(ctx context.Context, client Client, w io.Writer, spec *ssmversion.Spec, decrypt bool, jsonFormat bool) error {
	param, err := ssmversion.GetParameterWithVersion(ctx, client, spec, decrypt)
	if err != nil {
		return err
	}

	out := output.New(w)
	out.Field("Name", aws.ToString(param.Name))
	out.Field("Version", fmt.Sprintf("%d", param.Version))
	out.Field("Type", string(param.Type))
	if param.LastModifiedDate != nil {
		out.Field("Modified", param.LastModifiedDate.Format(time.RFC3339))
	}
	out.Separator()

	value := aws.ToString(param.Value)

	// Warn if --json is used in cases where it's not meaningful
	if jsonFormat {
		switch {
		case param.Type == types.ParameterTypeStringList:
			output.Warning(w, "--json has no effect on StringList type (comma-separated values)")
		case param.Type == types.ParameterTypeSecureString && !decrypt:
			output.Warning(w, "--json has no effect on encrypted SecureString (use --decrypt to enable)")
		case !jsonutil.IsJSON(value):
			output.Warning(w, "--json has no effect: value is not valid JSON")
		default:
			value = jsonutil.Format(value)
		}
	}
	out.Value(value)

	return nil
}
