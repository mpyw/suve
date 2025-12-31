// Package restore provides the SM restore command.
package restore

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

// Client is the interface for the restore command.
type Client interface {
	smapi.RestoreSecretAPI
}

// Runner executes the restore command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the restore command.
type Options struct {
	Name string
}

// Command returns the restore command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a deleted secret",
		ArgsUsage: "<name>",
		Description: `Restore a secret that was scheduled for deletion.

This only works for secrets that were deleted with a recovery window
and haven't been permanently deleted yet. Secrets deleted with --force
cannot be restored.

EXAMPLES:
   suve sm restore my-secret    Restore a deleted secret`,
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: suve sm restore <name>")
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: c.App.Writer,
		Stderr: c.App.ErrWriter,
	}
	return r.Run(c.Context, Options{
		Name: c.Args().First(),
	})
}

// Run executes the restore command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.Client.RestoreSecret(ctx, &secretsmanager.RestoreSecretInput{
		SecretId: aws.String(opts.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to restore secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Restored secret %s\n",
		green("✓"),
		aws.ToString(result.Name),
	)

	return nil
}
