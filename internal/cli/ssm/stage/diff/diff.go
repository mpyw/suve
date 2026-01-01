// Package diff provides the SSM stage diff command for comparing staged vs AWS values.
package diff

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

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
	Name       string // Parameter name (empty = all staged)
	JSONFormat bool
	NoPager    bool
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between staged and AWS values",
		ArgsUsage: "[name]",
		Description: `Compare staged values against AWS current values.

If a parameter name is specified, shows diff for that parameter only.
Otherwise, shows diff for all staged SSM parameters.

EXAMPLES:
   suve ssm stage diff              Show diff for all staged SSM parameters
   suve ssm stage diff /app/config  Show diff for specific parameter
   suve ssm stage diff -j           Show diff with JSON formatting`,
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
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	var name string
	if cmd.Args().Len() > 1 {
		return fmt.Errorf("usage: suve ssm stage diff [name]")
	}
	if cmd.Args().Len() == 1 {
		// Parse and validate the name (no version specifier allowed)
		spec, err := ssmversion.Parse(cmd.Args().First())
		if err != nil {
			return err
		}
		if spec.Absolute.Version != nil || spec.Shift > 0 {
			return fmt.Errorf("stage diff requires a parameter name without version specifier")
		}
		name = spec.Name
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
		Name:       name,
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
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

// fetchResult holds the result of fetching an SSM parameter.
type fetchResult struct {
	param *types.ParameterHistory
	err   error
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	// Get all staged entries for SSM
	allEntries, err := r.Store.List(stage.ServiceSSM)
	if err != nil {
		return err
	}
	entries := allEntries[stage.ServiceSSM]

	// Filter by name if specified
	if opts.Name != "" {
		entry, err := r.Store.Get(stage.ServiceSSM, opts.Name)
		if err == stage.ErrNotStaged {
			output.Warning(r.Stderr, "%s is not staged", opts.Name)
			return nil
		}
		if err != nil {
			return err
		}
		entries = map[string]stage.Entry{opts.Name: *entry}
	}

	if len(entries) == 0 {
		output.Warning(r.Stderr, "nothing staged")
		return nil
	}

	// Sort keys for consistent output
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	// Fetch all values in parallel
	results := make(map[string]*fetchResult)
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10) // Limit concurrent AWS API calls

	for name := range entries {
		g.Go(func() error {
			spec := &ssmversion.Spec{Name: name}
			param, err := ssmversion.GetParameterWithVersion(gctx, r.Client, spec, true)
			mu.Lock()
			results[name] = &fetchResult{param: param, err: err}
			mu.Unlock()
			return nil // Don't fail the group on individual errors
		})
	}

	_ = g.Wait() // Errors are tracked per-item

	// Output results in sorted order
	first := true
	for _, name := range names {
		entry := entries[name]
		result := results[name]

		if result.err != nil {
			return fmt.Errorf("failed to get current version for %s: %w", name, result.err)
		}

		if !first {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		first = false

		if err := r.outputDiff(opts, name, entry, result.param); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) outputDiff(opts Options, name string, entry stage.Entry, param *types.ParameterHistory) error {
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
			output.Warning(r.Stderr, "--json has no effect for %s: some values are not valid JSON", name)
		}
	}

	if awsValue == stagedValue {
		// Auto-unstage since there's no difference
		if err := r.Store.Unstage(stage.ServiceSSM, name); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}
		output.Warning(r.Stderr, "unstaged %s: identical to AWS current", name)
		return nil
	}

	label1 := fmt.Sprintf("%s#%d (AWS)", name, param.Version)
	label2 := fmt.Sprintf("%s (staged)", name)
	if entry.Operation == stage.OperationDelete {
		label2 = fmt.Sprintf("%s (staged for deletion)", name)
	}

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}
