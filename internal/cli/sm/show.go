package sm

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version"
)

func showCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show secret value with metadata",
		ArgsUsage: "<name[@version][~shift][:label]>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Pretty print JSON values",
			},
		},
		Action: showAction,
	}
}

func showAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Show(ctx, client, c.App.Writer, spec, c.Bool("json"))
}

// Show displays secret value with metadata.
func Show(ctx context.Context, client ShowClient, w io.Writer, spec *version.Spec, prettyJSON bool) error {
	secret, err := GetSecretWithVersion(ctx, client, spec)
	if err != nil {
		return err
	}

	out := output.New(w)
	out.Field("Name", aws.ToString(secret.Name))
	out.Field("ARN", aws.ToString(secret.ARN))
	if secret.VersionId != nil {
		out.Field("VersionId", aws.ToString(secret.VersionId))
	}
	if len(secret.VersionStages) > 0 {
		out.Field("Stages", fmt.Sprintf("%v", secret.VersionStages))
	}
	if secret.CreatedDate != nil {
		out.Field("Created", secret.CreatedDate.Format(time.RFC3339))
	}
	out.Separator()

	value := aws.ToString(secret.SecretString)
	if prettyJSON {
		value = formatJSON(value)
	}
	out.Value(value)

	return nil
}

// VersionedClient is the interface for commands that need version support.
type VersionedClient interface {
	GetSecretValueAPI
	ListSecretVersionIdsAPI
}

// GetSecretWithVersion retrieves a secret with version/shift/label support.
func GetSecretWithVersion(ctx context.Context, client VersionedClient, spec *version.Spec) (*secretsmanager.GetSecretValueOutput, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(spec.Name),
	}

	if spec.Label != nil {
		input.VersionStage = spec.Label
	}

	if spec.HasShift() {
		versions, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
			SecretId: aws.String(spec.Name),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list versions: %w", err)
		}

		versionList := versions.Versions
		sort.Slice(versionList, func(i, j int) bool {
			if versionList[i].CreatedDate == nil {
				return false
			}
			if versionList[j].CreatedDate == nil {
				return true
			}
			return versionList[i].CreatedDate.After(*versionList[j].CreatedDate)
		})

		if spec.Shift >= len(versionList) {
			return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
		}

		targetVersion := versionList[spec.Shift]
		input.VersionId = targetVersion.VersionId
	}

	return client.GetSecretValue(ctx, input)
}
