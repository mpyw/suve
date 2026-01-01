// Package delete provides the SM delete command.
package delete

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/confirm"
)

// Client is the interface for the delete command.
type Client interface {
	smapi.DeleteSecretAPI
}

// Runner executes the delete command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the delete command.
type Options struct {
	Name           string
	Force          bool
	RecoveryWindow int
}

// Command returns the delete command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "delete",
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
   suve sm delete my-secret                      Delete with 30-day recovery (with confirmation)
   suve sm delete --recovery-window 7 my-secret  Delete with 7-day recovery
   suve sm delete --force my-secret              Permanently delete immediately
   suve sm delete -y my-secret                   Delete without confirmation`,
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
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	// Confirm deletion
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
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
		Name:           name,
		Force:          cmd.Bool("force"),
		RecoveryWindow: int(cmd.Int("recovery-window")),
	})
}

// Run executes the delete command.
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
