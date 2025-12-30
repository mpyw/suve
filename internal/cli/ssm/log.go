package ssm

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/output"
)

func logCommand() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Usage:     "Show parameter version history",
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
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))

	ctx := context.Background()
	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Log(ctx, client, c.App.Writer, name, maxResults)
}

// Log displays parameter version history.
func Log(ctx context.Context, client LogClient, w io.Writer, name string, maxResults int32) error {
	result, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(name),
		MaxResults:     aws.Int32(maxResults),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter history: %w", err)
	}

	out := output.New(w)
	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	for i, param := range result.Parameters {
		versionLabel := fmt.Sprintf("Version %d", param.Version)
		if i == 0 {
			versionLabel += " (current)"
		}
		_, _ = fmt.Fprintln(w, yellow(versionLabel))
		if param.LastModifiedDate != nil {
			_, _ = fmt.Fprintf(w, "%s %s\n", cyan("Date:"), param.LastModifiedDate.Format(time.RFC3339))
		}
		if param.LastModifiedUser != nil {
			_, _ = fmt.Fprintf(w, "%s %s\n", cyan("User:"), aws.ToString(param.LastModifiedUser))
		}
		out.ValuePreview(aws.ToString(param.Value), 100)
		if i < len(result.Parameters)-1 {
			_, _ = fmt.Fprintln(w)
		}
	}

	return nil
}
