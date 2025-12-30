// Package log provides the SSM log command.
package log

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the log command.
type Client interface {
	ssmapi.GetParameterHistoryAPI
}

// Command returns the log command.
func Command() *cli.Command {
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
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, maxResults)
}

// Run executes the log command.
func Run(ctx context.Context, client Client, w io.Writer, name string, maxResults int32) error {
	result, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(name),
		MaxResults:     aws.Int32(maxResults),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter history: %w", err)
	}

	// Reverse to show newest first
	params := result.Parameters
	for i, j := 0, len(params)-1; i < j; i, j = i+1, j-1 {
		params[i], params[j] = params[j], params[i]
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	for i, param := range params {
		versionLabel := fmt.Sprintf("Version %d", param.Version)
		if i == 0 {
			versionLabel += " " + green("(current)")
		}
		_, _ = fmt.Fprintln(w, yellow(versionLabel))
		if param.LastModifiedDate != nil {
			_, _ = fmt.Fprintf(w, "%s %s\n", cyan("Date:"), param.LastModifiedDate.Format(time.RFC3339))
		}

		// Show truncated value preview
		value := aws.ToString(param.Value)
		if len(value) > 50 {
			value = value[:50] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\n", value)

		if i < len(params)-1 {
			_, _ = fmt.Fprintln(w)
		}
	}

	return nil
}
