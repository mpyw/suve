// Package reset provides the SSM reset command for unstaging or restoring parameters.
package reset

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the reset command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
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
	Spec string // Parameter name with optional version spec
	All  bool   // Reset all staged SSM parameters
}

// Command returns the reset command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "reset",
		Usage:     "Unstage parameter or restore to specific version",
		ArgsUsage: "[spec]",
		Description: `Remove a parameter from staging area or restore to a specific version.

Without a version specifier, the parameter is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use 'suve ssm reset --all' to unstage all SSM parameters at once.

VERSION SPECIFIERS:
   /app/config          Unstage parameter (remove from staging)
   /app/config#3        Restore to version 3
   /app/config~1        Restore to 1 version ago

EXAMPLES:
   suve ssm reset /app/config              Unstage (remove from staging)
   suve ssm reset /app/config#3            Stage value from version 3
   suve ssm reset /app/config~1            Stage value from previous version
   suve ssm reset --all                    Unstage all SSM parameters`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all SSM parameters",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	resetAll := cmd.Bool("all")

	if !resetAll && cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm stage reset <spec> or suve ssm stage reset --all")
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
		spec, err := ssmversion.Parse(opts.Spec)
		if err != nil {
			return err
		}
		needsAWS = spec.Absolute.Version != nil || spec.Shift > 0
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	if needsAWS {
		client, err := awsutil.NewSSMClient(ctx)
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

	spec, err := ssmversion.Parse(opts.Spec)
	if err != nil {
		return err
	}

	// If version or shift specified, restore to that version
	if spec.Absolute.Version != nil || spec.Shift > 0 {
		return r.runRestore(ctx, spec)
	}

	// Otherwise, just unstage
	return r.runUnstage(spec.Name)
}

func (r *Runner) runUnstageAll() error {
	// Check if there are any staged changes
	staged, err := r.Store.List(stage.ServiceSSM)
	if err != nil {
		return err
	}

	ssmStaged := staged[stage.ServiceSSM]
	if len(ssmStaged) == 0 {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No SSM changes staged."))
		return nil
	}

	if err := r.Store.UnstageAll(stage.ServiceSSM); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all SSM parameters (%d)\n", green("✓"), len(ssmStaged))
	return nil
}

func (r *Runner) runUnstage(name string) error {
	// Check if actually staged
	_, err := r.Store.Get(stage.ServiceSSM, name)
	if err == stage.ErrNotStaged {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintf(r.Stdout, "%s %s is not staged\n", yellow("!"), name)
		return nil
	}
	if err != nil {
		return err
	}

	if err := r.Store.Unstage(stage.ServiceSSM, name); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged %s\n", green("✓"), name)
	return nil
}

func (r *Runner) runRestore(ctx context.Context, spec *ssmversion.Spec) error {
	// Fetch the specific version
	param, err := ssmversion.GetParameterWithVersion(ctx, r.Client, spec, true)
	if err != nil {
		return err
	}

	value := lo.FromPtr(param.Value)

	// Stage this value
	if err := r.Store.Stage(stage.ServiceSSM, spec.Name, stage.Entry{
		Operation: stage.OperationSet,
		Value:     value,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Restored %s (staged from version %d)\n",
		green("✓"), spec.Name, param.Version)
	return nil
}
