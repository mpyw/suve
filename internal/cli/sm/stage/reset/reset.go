// Package reset provides the SM reset command for unstaging or restoring secrets.
package reset

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the reset command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the reset command.
type Runner struct {
	Client Client
	Store  *stage.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the reset command.
type Options struct {
	Spec string // Secret name with optional version spec
	All  bool   // Reset all staged SM secrets
}

// Command returns the reset command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "reset",
		Usage:     "Unstage secret or restore to specific version",
		ArgsUsage: "[spec]",
		Description: `Remove a secret from staging area or restore to a specific version.

Without a version specifier, the secret is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use 'suve sm reset --all' to unstage all SM secrets at once.

VERSION SPECIFIERS:
   my-secret            Unstage secret (remove from staging)
   my-secret#abc123     Restore to specific version ID
   my-secret:AWSPREVIOUS  Restore to AWSPREVIOUS label
   my-secret~1          Restore to 1 version ago

EXAMPLES:
   suve sm reset my-secret              Unstage (remove from staging)
   suve sm reset my-secret#abc123       Stage value from specific version
   suve sm reset my-secret:AWSPREVIOUS  Stage value from AWSPREVIOUS
   suve sm reset my-secret~1            Stage value from previous version
   suve sm reset --all                  Unstage all SM secrets`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all SM secrets",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	resetAll := cmd.Bool("all")

	if !resetAll && cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm stage reset <spec> or suve sm stage reset --all")
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	opts := Options{
		All: resetAll,
	}
	if !resetAll {
		opts.Spec = cmd.Args().First()
	}

	// Check if version spec is provided (need AWS client)
	needsAWS := false
	if !resetAll && opts.Spec != "" {
		spec, err := smversion.Parse(opts.Spec)
		if err != nil {
			return err
		}
		needsAWS = spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	if needsAWS {
		client, err := awsutil.NewSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize AWS client: %w", err)
		}
		r.Client = client
	}

	return r.Run(ctx, opts)
}

// Run executes the reset command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	if opts.All {
		return r.runUnstageAll()
	}

	spec, err := smversion.Parse(opts.Spec)
	if err != nil {
		return err
	}

	// If version, label, or shift specified, restore to that version
	if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
		return r.runRestore(ctx, spec)
	}

	// Otherwise, just unstage
	return r.runUnstage(spec.Name)
}

func (r *Runner) runUnstageAll() error {
	// Check if there are any staged changes
	staged, err := r.Store.List(stage.ServiceSM)
	if err != nil {
		return err
	}

	smStaged := staged[stage.ServiceSM]
	if len(smStaged) == 0 {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No SM changes staged."))
		return nil
	}

	if err := r.Store.UnstageAll(stage.ServiceSM); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all SM secrets (%d)\n", green("✓"), len(smStaged))
	return nil
}

func (r *Runner) runUnstage(name string) error {
	// Check if actually staged
	_, err := r.Store.Get(stage.ServiceSM, name)
	if err == stage.ErrNotStaged {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintf(r.Stdout, "%s %s is not staged\n", yellow("!"), name)
		return nil
	}
	if err != nil {
		return err
	}

	if err := r.Store.Unstage(stage.ServiceSM, name); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged %s\n", green("✓"), name)
	return nil
}

func (r *Runner) runRestore(ctx context.Context, spec *smversion.Spec) error {
	// Fetch the specific version
	secret, err := smversion.GetSecretWithVersion(ctx, r.Client, spec)
	if err != nil {
		return err
	}

	value := lo.FromPtr(secret.SecretString)

	// Stage this value
	if err := r.Store.Stage(stage.ServiceSM, spec.Name, stage.Entry{
		Operation: stage.OperationSet,
		Value:     value,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Restored %s (staged from version %s)\n",
		green("✓"), spec.Name, lo.FromPtr(secret.VersionId))
	return nil
}
