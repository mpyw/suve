package sm

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
)

func logCommand() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Usage:     "Show secret version history",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10,
				Usage:   "Number of versions to show",
			},
		},
		Action: logAction,
	}
}

func logAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))

	ctx := context.Background()
	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Log(ctx, client, c.App.Writer, name, maxResults)
}

// Log displays secret version history.
func Log(ctx context.Context, client LogClient, w io.Writer, name string, maxResults int32) error {
	result, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId:   aws.String(name),
		MaxResults: aws.Int32(maxResults),
	})
	if err != nil {
		return fmt.Errorf("failed to list secret versions: %w", err)
	}

	versions := result.Versions
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedDate == nil {
			return false
		}
		if versions[j].CreatedDate == nil {
			return true
		}
		return versions[i].CreatedDate.After(*versions[j].CreatedDate)
	})

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	for i, v := range versions {
		versionLabel := fmt.Sprintf("Version %s", aws.ToString(v.VersionId)[:8])
		if len(v.VersionStages) > 0 {
			versionLabel += " " + green(fmt.Sprintf("%v", v.VersionStages))
		}
		_, _ = fmt.Fprintln(w, yellow(versionLabel))
		if v.CreatedDate != nil {
			_, _ = fmt.Fprintf(w, "%s %s\n", cyan("Date:"), v.CreatedDate.Format(time.RFC3339))
		}
		if i < len(versions)-1 {
			_, _ = fmt.Fprintln(w)
		}
	}

	return nil
}
