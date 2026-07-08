// Package delete provides the Secrets Manager delete command.
package delete

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/provider"
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Recovery-window bounds enforced by AWS Secrets Manager, mirrored here so an
// invalid value is rejected before the confirmation prompt rather than by AWS
// after it.
const (
	minRecoveryWindow = 7
	maxRecoveryWindow = 30
	// defaultRecoveryWindow is the AWS Secrets Manager default recovery window
	// in days, used to compute the displayed scheduled deletion date when no
	// explicit window is given.
	defaultRecoveryWindow = maxRecoveryWindow
)

// validateDeleteFlags checks the --force / --recovery-window combination before
// any confirmation prompt or deletion is attempted. recoveryWindow must be the
// value the user explicitly supplied (0 when the flag was left at its default),
// so that --force alone is not mistaken for a combined invocation.
func validateDeleteFlags(force bool, recoveryWindow int) error {
	if force && recoveryWindow > 0 {
		return fmt.Errorf("--force and --recovery-window cannot be combined")
	}

	if recoveryWindow > 0 && (recoveryWindow < minRecoveryWindow || recoveryWindow > maxRecoveryWindow) {
		return fmt.Errorf("--recovery-window must be between %d and %d days", minRecoveryWindow, maxRecoveryWindow)
	}

	return nil
}

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
				Value: defaultRecoveryWindow,
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
	force := cmd.Bool("force")
	recoveryWindow := cmd.Int("recovery-window")

	// Only the explicitly-supplied window participates in flag validation; the
	// flag default must not be mistaken for a user-supplied value.
	givenWindow := 0
	if cmd.IsSet("recovery-window") {
		givenWindow = recoveryWindow
	}

	if err := validateDeleteFlags(force, givenWindow); err != nil {
		return err
	}

	store, err := internal.SecretStore(ctx)
	if err != nil {
		return err
	}

	// Get AWS identity for confirmation display
	var identity *infra.AWSIdentity
	if !skipConfirm {
		identity, _ = infra.GetAWSIdentity(ctx)
	}

	uc := &secret.DeleteUseCase{Store: store}

	// Show current value before confirming
	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			output.Info(cmd.Root().ErrWriter, "Current value of %s:", name)
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
		Force:          force,
		RecoveryWindow: recoveryWindow,
	})
}

// Run executes the delete command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	var options []provider.DeleteOption

	switch {
	case opts.Force:
		options = append(options, awssecret.ForceDelete{})
	case opts.RecoveryWindow > 0:
		options = append(options, awssecret.RecoveryWindow{Days: int64(opts.RecoveryWindow)})
	}

	result, err := r.UseCase.Execute(ctx, secret.DeleteInput{
		Name:    opts.Name,
		Options: options,
	})
	if err != nil {
		return err
	}

	if opts.Force {
		output.Success(r.Stdout, "Permanently deleted secret %s", result.Name)

		return nil
	}

	// The provider Delete returns only an error, so the scheduled deletion date
	// is computed client-side (now + recovery window) — the same calendar date
	// AWS itself schedules.
	window := opts.RecoveryWindow
	if window <= 0 {
		window = defaultRecoveryWindow
	}

	deletionDate := time.Now().AddDate(0, 0, window)

	output.Success(r.Stdout, "Scheduled deletion of secret %s (deletion date: %s)",
		result.Name,
		deletionDate.Format("2006-01-02"),
	)

	return nil
}
