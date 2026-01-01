// Package rm provides the SM rm command.
package rm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the rm command.
type Client interface {
	smapi.DeleteSecretAPI
}

// Runner executes the rm command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the rm command.
type Options struct {
	Name           string
	Force          bool
	RecoveryWindow int
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
   suve sm rm --force my-secret              Permanently delete immediately`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Force deletion without recovery window",
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

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm rm <name>")
	}

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name:           cmd.Args().First(),
		Force:          cmd.Bool("force"),
		RecoveryWindow: int(cmd.Int("recovery-window")),
	})
}

// Run executes the rm command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: lo.ToPtr(opts.Name),
	}

	if opts.Force {
		input.ForceDeleteWithoutRecovery = lo.ToPtr(true)
	} else {
		input.RecoveryWindowInDays = lo.ToPtr(int64(opts.RecoveryWindow))
	}

	result, err := r.Client.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	if opts.Force {
		_, _ = fmt.Fprintf(r.Stdout, "%s Permanently deleted secret %s\n",
			yellow("!"),
			lo.FromPtr(result.Name),
		)
	} else {
		_, _ = fmt.Fprintf(r.Stdout, "%s Scheduled deletion of secret %s (deletion date: %s)\n",
			yellow("!"),
			lo.FromPtr(result.Name),
			result.DeletionDate.Format("2006-01-02"),
		)
	}

	return nil
}
