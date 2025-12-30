// Package log provides the SM log command.
package log

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

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the log command.
type Client interface {
	smapi.ListSecretVersionIdsAPI
}

// Command returns the log command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Usage:     "Show secret version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a secret, showing each version's
UUID (truncated), staging labels, and creation date.

Output is sorted with the most recent version first.
Version UUIDs are truncated to 8 characters for readability.

EXAMPLES:
   suve sm log my-secret           Show last 10 versions (default)
   suve sm log -n 5 my-secret      Show last 5 versions
   suve sm log -n 50 my-secret     Show last 50 versions`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10,
				Usage:   "Number of versions to show",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, maxResults)
}

// Run executes the log command.
func Run(ctx context.Context, client Client, w io.Writer, name string, maxResults int32) error {
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
		versionID := aws.ToString(v.VersionId)
		if len(versionID) > 8 {
			versionID = versionID[:8]
		}
		versionLabel := fmt.Sprintf("Version %s", versionID)
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
