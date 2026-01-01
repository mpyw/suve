// Package diff provides the SSM diff command for comparing parameter versions.
package diff

import (
	"context"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the diff command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// Runner executes the diff command.
type Runner struct {
	Client Client
	Store  *stage.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	Spec1      *ssmversion.Spec
	Spec2      *ssmversion.Spec
	JSONFormat bool
	NoPager    bool
	Staged     bool   // Compare staged value vs AWS current
	StagedName string // Parameter name for staged diff
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> <version1> [version2]",
		Description: `Compare two versions of a parameter in unified diff format.
If only one version/spec is specified, compares against latest.

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve ssm diff /app/config#1 /app/config#2   Compare v1 and v2 (full spec)
  suve ssm diff /app/config#3                 Compare v3 with latest (full spec)
  suve ssm diff /app/config#1 '#2'            Compare v1 and v2 (mixed)
  suve ssm diff /app/config '#1' '#2'         Compare v1 and v2 (partial spec)
  suve ssm diff /app/config '~'               Compare previous with latest
  suve ssm diff -j /app/config#1 /app/config  JSON format before diffing
  suve ssm diff --staged /app/config          Compare staged vs AWS current`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.BoolFlag{
				Name:  "staged",
				Usage: "Compare staged value vs AWS current value",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	staged := cmd.Bool("staged")

	// Handle --staged mode
	if staged {
		if cmd.Args().Len() != 1 {
			return fmt.Errorf("usage: suve ssm diff --staged <name>")
		}

		// Parse and validate the name (no version specifier allowed)
		spec, err := ssmversion.Parse(cmd.Args().First())
		if err != nil {
			return err
		}
		if spec.Absolute.Version != nil || spec.Shift > 0 {
			return fmt.Errorf("--staged requires a parameter name without version specifier")
		}

		store, err := stage.NewStore()
		if err != nil {
			return fmt.Errorf("failed to initialize stage store: %w", err)
		}

		client, err := awsutil.NewSSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize AWS client: %w", err)
		}

		opts := Options{
			JSONFormat: cmd.Bool("json"),
			NoPager:    cmd.Bool("no-pager"),
			Staged:     true,
			StagedName: spec.Name,
		}

		return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
			r := &Runner{
				Client: client,
				Store:  store,
				Stdout: w,
				Stderr: cmd.Root().ErrWriter,
			}
			return r.Run(ctx, opts)
		})
	}

	// Normal diff mode
	spec1, spec2, err := ssmversion.ParseDiffArgs(cmd.Args().Slice())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec1:      spec1,
		Spec2:      spec2,
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r := &Runner{
			Client: client,
			Stdout: w,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	if opts.Staged {
		return r.runStaged(ctx, opts)
	}
	return r.runNormal(ctx, opts)
}

func (r *Runner) runStaged(ctx context.Context, opts Options) error {
	// Get staged value
	entry, err := r.Store.Get(stage.ServiceSSM, opts.StagedName)
	if err == stage.ErrNotStaged {
		output.Warning(r.Stderr, "%s is not staged", opts.StagedName)
		return nil
	}
	if err != nil {
		return err
	}

	// Get current AWS value
	spec := &ssmversion.Spec{Name: opts.StagedName}
	param, err := ssmversion.GetParameterWithVersion(ctx, r.Client, spec, true)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	awsValue := lo.FromPtr(param.Value)
	stagedValue := entry.Value

	// For delete operation, staged value is empty
	if entry.Operation == stage.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.JSONFormat {
		formatted1, ok1 := jsonutil.TryFormat(awsValue)
		formatted2, ok2 := jsonutil.TryFormat(stagedValue)
		if ok1 && ok2 {
			awsValue = formatted1
			stagedValue = formatted2
		} else if ok1 || ok2 {
			output.Warning(r.Stderr, "--json has no effect: some values are not valid JSON")
		}
	}

	if awsValue == stagedValue {
		output.Warning(r.Stderr, "staged value is identical to AWS current")
		return nil
	}

	label1 := fmt.Sprintf("%s#%d (AWS)", opts.StagedName, param.Version)
	label2 := fmt.Sprintf("%s (staged)", opts.StagedName)
	if entry.Operation == stage.OperationDelete {
		label2 = fmt.Sprintf("%s (staged for deletion)", opts.StagedName)
	}

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}

func (r *Runner) runNormal(ctx context.Context, opts Options) error {
	param1, err := ssmversion.GetParameterWithVersion(ctx, r.Client, opts.Spec1, true)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	param2, err := ssmversion.GetParameterWithVersion(ctx, r.Client, opts.Spec2, true)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := lo.FromPtr(param1.Value)
	value2 := lo.FromPtr(param2.Value)

	// Format as JSON if enabled
	if opts.JSONFormat {
		formatted1, ok1 := jsonutil.TryFormat(value1)
		formatted2, ok2 := jsonutil.TryFormat(value2)
		if ok1 && ok2 {
			value1 = formatted1
			value2 = formatted2
		} else {
			output.Warning(r.Stderr, "--json has no effect: some values are not valid JSON")
		}
	}

	if value1 == value2 {
		output.Warning(r.Stderr, "comparing identical versions")
		output.Hint(r.Stderr, "To compare with previous version, use: suve ssm diff %s~1", opts.Spec1.Name)
		return nil
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%d", opts.Spec1.Name, param1.Version),
		fmt.Sprintf("%s#%d", opts.Spec2.Name, param2.Version),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}
