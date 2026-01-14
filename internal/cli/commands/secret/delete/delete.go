// Package delete provides the Secrets Manager delete command.
package delete

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the delete command.
type Runner struct {
	UseCase *secret.DeleteUseCase
	Stdout  io.Writer
	Stderr  io.Writer
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
		Aliases:   []string{"rm"},
		Usage:     "Delete a secret",
		ArgsUsage: "<name>",
		Description: `Schedule a secret for deletion in AWS Secrets Manager.

By default, secrets are scheduled for deletion after a 30-day recovery
window. During this period, you can restore the secret using 'suve secret restore'.

Use --force for immediate permanent deletion without a recovery window.
This action cannot be undone.

RECOVERY WINDOW:
   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   suve secret delete my-secret                      Delete with 30-day recovery (with confirmation)
   suve secret delete --recovery-window 7 my-secret  Delete with 7-day recovery
   suve secret delete --force my-secret              Permanently delete immediately
   suve secret delete --yes my-secret                Delete without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Force deletion without recovery window",
			},
			&cli.IntFlag{
				Name:  "recovery-window",
				Usage: "Number of days before permanent deletion (7-30)",
				Value: 30, //nolint:mnd // AWS Secrets Manager default recovery window
			},
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve secret delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// Get AWS identity for confirmation display
	var identity *infra.AWSIdentity
	if !skipConfirm {
		identity, _ = infra.GetAWSIdentity(ctx)
	}

	uc := &secret.DeleteUseCase{Client: client}

	// Show current value before confirming
	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			output.Warn(cmd.Root().ErrWriter, "Current value of %s:", name)
			output.Println(cmd.Root().ErrWriter, "")
			output.Println(cmd.Root().ErrWriter, output.Indent(currentValue, "  "))
			output.Println(cmd.Root().ErrWriter, "")
		}
	}

	// Confirm deletion
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	if identity != nil {
		prompter.AccountID = identity.AccountID
		prompter.Region = identity.Region
		prompter.Profile = identity.Profile
	}

	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &Runner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name:           name,
		Force:          cmd.Bool("force"),
		RecoveryWindow: cmd.Int("recovery-window"),
	})
}

// Run executes the delete command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.DeleteInput{
		Name:           opts.Name,
		Force:          opts.Force,
		RecoveryWindow: int64(opts.RecoveryWindow),
	})
	if err != nil {
		return err
	}

	if opts.Force {
		output.Warn(r.Stdout, "Permanently deleted secret %s", result.Name)
	} else {
		output.Warn(r.Stdout, "Scheduled deletion of secret %s (deletion date: %s)",
			result.Name,
			result.DeletionDate.Format("2006-01-02"),
		)
	}

	return nil
}
