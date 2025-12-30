// Package rm provides the SM rm command.
package rm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the rm command.
type Client interface {
	smapi.DeleteSecretAPI
}

// Command returns the rm command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "Delete a secret",
		ArgsUsage: "<name>",
		Description: `Schedule a secret for deletion in AWS Secrets Manager.

By default, secrets are scheduled for deletion after a 30-day recovery
window. During this period, you can restore the secret using 'suve sm restore'.

Use --force for immediate permanent deletion without a recovery window.
This action cannot be undone.

RECOVERY WINDOW:
   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   suve sm rm my-secret                      Delete with 30-day recovery
   suve sm rm --recovery-window 7 my-secret  Delete with 7-day recovery
   suve sm rm -f my-secret                   Permanently delete immediately`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Force deletion without recovery window",
			},
			&cli.IntFlag{
				Name:  "recovery-window",
				Usage: "Number of days before permanent deletion (7-30)",
				Value: 30,
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: suve sm rm <name>")
	}

	name := c.Args().First()
	force := c.Bool("force")
	recoveryWindow := c.Int("recovery-window")

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, force, recoveryWindow)
}

// Run executes the rm command.
func Run(ctx context.Context, client Client, w io.Writer, name string, force bool, recoveryWindow int) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: aws.String(name),
	}

	if force {
		input.ForceDeleteWithoutRecovery = aws.Bool(true)
	} else {
		input.RecoveryWindowInDays = aws.Int64(int64(recoveryWindow))
	}

	result, err := client.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	if force {
		_, _ = fmt.Fprintf(w, "%s Permanently deleted secret %s\n",
			yellow("!"),
			aws.ToString(result.Name),
		)
	} else {
		_, _ = fmt.Fprintf(w, "%s Scheduled deletion of secret %s (deletion date: %s)\n",
			yellow("!"),
			aws.ToString(result.Name),
			result.DeletionDate.Format("2006-01-02"),
		)
	}

	return nil
}
