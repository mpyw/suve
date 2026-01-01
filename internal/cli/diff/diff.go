// Package diff provides the global diff command for viewing staged changes.
package diff

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/smutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/smversion"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// SSMClient is the interface for SSM operations.
type SSMClient interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// SMClient is the interface for SM operations.
type SMClient interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the diff command.
type Runner struct {
	SSMClient SSMClient
	SMClient  SMClient
	Store     *stage.Store
	Stdout    io.Writer
	Stderr    io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	JSONFormat bool
	NoPager    bool
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "diff",
		Usage: "Show diff of all staged changes (SSM and SM)",
		Description: `Compare all staged changes against AWS current values.

For comparing specific versions, use 'suve ssm diff' or 'suve sm diff'.

EXAMPLES:
   suve stage diff     Show diff of all staged changes
   suve stage diff -j  Show diff with JSON formatting`,
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
	if cmd.Args().Len() > 0 {
		return fmt.Errorf("usage: suve stage diff (no arguments)")
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	// Check if there are any staged changes before creating clients
	ssmStaged, err := store.List(stage.ServiceSSM)
	if err != nil {
		return err
	}
	smStaged, err := store.List(stage.ServiceSM)
	if err != nil {
		return err
	}

	hasSSM := len(ssmStaged[stage.ServiceSSM]) > 0
	hasSM := len(smStaged[stage.ServiceSM]) > 0

	if !hasSSM && !hasSM {
		output.Warning(cmd.Root().ErrWriter, "nothing staged")
		return nil
	}

	r := &Runner{
		Store:  store,
		Stderr: cmd.Root().ErrWriter,
	}

	// Initialize clients only if needed
	if hasSSM {
		ssmClient, err := awsutil.NewSSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize SSM client: %w", err)
		}
		r.SSMClient = ssmClient
	}

	if hasSM {
		smClient, err := awsutil.NewSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize SM client: %w", err)
		}
		r.SMClient = smClient
	}

	opts := Options{
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r.Stdout = w
		return r.Run(ctx, opts)
	})
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	allEntries, err := r.Store.List("")
	if err != nil {
		return err
	}

	if len(allEntries) == 0 {
		output.Warning(r.Stderr, "nothing staged")
		return nil
	}

	first := true

	// Process SSM entries
	if ssmEntries, ok := allEntries[stage.ServiceSSM]; ok && len(ssmEntries) > 0 {
		names := make([]string, 0, len(ssmEntries))
		for name := range ssmEntries {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			entry := ssmEntries[name]

			if !first {
				_, _ = fmt.Fprintln(r.Stdout)
			}
			first = false

			if err := r.diffSSMStaged(ctx, opts, name, entry); err != nil {
				return err
			}
		}
	}

	// Process SM entries
	if smEntries, ok := allEntries[stage.ServiceSM]; ok && len(smEntries) > 0 {
		names := make([]string, 0, len(smEntries))
		for name := range smEntries {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			entry := smEntries[name]

			if !first {
				_, _ = fmt.Fprintln(r.Stdout)
			}
			first = false

			if err := r.diffSMStaged(ctx, opts, name, entry); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Runner) diffSSMStaged(ctx context.Context, opts Options, name string, entry stage.Entry) error {
	// Get current AWS value
	spec := &ssmversion.Spec{Name: name}
	param, err := ssmversion.GetParameterWithVersion(ctx, r.SSMClient, spec, true)
	if err != nil {
		return fmt.Errorf("failed to get current version for %s: %w", name, err)
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
			output.Warning(r.Stderr, "--json has no effect for %s: some values are not valid JSON", name)
		}
	}

	if awsValue == stagedValue {
		output.Warning(r.Stderr, "staged value is identical to AWS current for %s", name)
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

func (r *Runner) diffSMStaged(ctx context.Context, opts Options, name string, entry stage.Entry) error {
	// Get current AWS value
	spec := &smversion.Spec{Name: name}
	secret, err := smversion.GetSecretWithVersion(ctx, r.SMClient, spec)
	if err != nil {
		return fmt.Errorf("failed to get current version for %s: %w", name, err)
	}

	awsValue := lo.FromPtr(secret.SecretString)
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
		output.Warning(r.Stderr, "staged value is identical to AWS current for %s", name)
		return nil
	}

	versionID := smutil.TruncateVersionID(lo.FromPtr(secret.VersionId))
	label1 := fmt.Sprintf("%s#%s (AWS)", name, versionID)
	label2 := fmt.Sprintf("%s (staged)", name)
	if entry.Operation == stage.OperationDelete {
		label2 = fmt.Sprintf("%s (staged for deletion)", name)
	}

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}
