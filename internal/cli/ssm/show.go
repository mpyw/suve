package ssm

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version"
)

func showCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show parameter value with metadata",
		ArgsUsage: "<name[@version][~shift]>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "decrypt",
				Aliases: []string{"d"},
				Value:   true,
				Usage:   "Decrypt SecureString values",
			},
		},
		Action: showAction,
	}
}

func showAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Show(ctx, client, c.App.Writer, spec, c.Bool("decrypt"))
}

// Show displays parameter value with metadata.
func Show(ctx context.Context, client ShowClient, w io.Writer, spec *version.Spec, decrypt bool) error {
	param, err := GetParameterWithVersion(ctx, client, spec, decrypt)
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
	out.Value(aws.ToString(param.Value))

	return nil
}

// VersionedClient is the interface for commands that need version support.
type VersionedClient interface {
	GetParameterAPI
	GetParameterHistoryAPI
}

// GetParameterWithVersion retrieves a parameter with version/shift support.
func GetParameterWithVersion(ctx context.Context, client VersionedClient, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	if spec.HasShift() {
		return getParameterWithShift(ctx, client, spec, decrypt)
	}
	return getParameterDirect(ctx, client, spec, decrypt)
}

func getParameterWithShift(ctx context.Context, client GetParameterHistoryAPI, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	history, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(spec.Name),
		WithDecryption: aws.Bool(decrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	if len(history.Parameters) == 0 {
		return nil, fmt.Errorf("parameter not found: %s", spec.Name)
	}

	// Reverse to get newest first
	params := history.Parameters
	for i, j := 0, len(params)-1; i < j; i, j = i+1, j-1 {
		params[i], params[j] = params[j], params[i]
	}

	baseIdx := 0
	if spec.Version != nil {
		found := false
		for i, p := range params {
			if p.Version == *spec.Version {
				baseIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("version %d not found", *spec.Version)
		}
	}

	targetIdx := baseIdx + spec.Shift
	if targetIdx >= len(params) {
		return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
	}

	return &params[targetIdx], nil
}

func getParameterDirect(ctx context.Context, client GetParameterAPI, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	var nameWithVersion string
	if spec.Version != nil {
		nameWithVersion = fmt.Sprintf("%s:%d", spec.Name, *spec.Version)
	} else {
		nameWithVersion = spec.Name
	}

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(nameWithVersion),
		WithDecryption: aws.Bool(decrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	param := result.Parameter
	return &types.ParameterHistory{
		Name:             param.Name,
		Value:            param.Value,
		Type:             param.Type,
		Version:          param.Version,
		LastModifiedDate: param.LastModifiedDate,
	}, nil
}
