// Package diff provides the global diff command for viewing staged changes.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
	// smutil removed
	"github.com/mpyw/suve/internal/staging"
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
	Store     *staging.Store
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

	store, err := staging.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	// Check if there are any staged changes before creating clients
	ssmStaged, err := store.List(staging.ServiceSSM)
	if err != nil {
		return err
	}
	smStaged, err := store.List(staging.ServiceSM)
	if err != nil {
		return err
	}

	hasSSM := len(ssmStaged[staging.ServiceSSM]) > 0
	hasSM := len(smStaged[staging.ServiceSM]) > 0

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
		ssmClient, err := infra.NewSSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize SSM client: %w", err)
		}
		r.SSMClient = ssmClient
	}

	if hasSM {
		smClient, err := infra.NewSMClient(ctx)
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

	ssmEntries := allEntries[staging.ServiceSSM]
	smEntries := allEntries[staging.ServiceSM]

	// Fetch all values in parallel
	ssmResults := parallel.ExecuteMap(ctx, ssmEntries, func(ctx context.Context, name string, _ staging.Entry) (*types.ParameterHistory, error) {
		spec := &ssmversion.Spec{Name: name}
		return ssmversion.GetParameterWithVersion(ctx, r.SSMClient, spec, true)
	})

	smResults := parallel.ExecuteMap(ctx, smEntries, func(ctx context.Context, name string, _ staging.Entry) (*secretsmanager.GetSecretValueOutput, error) {
		spec := &smversion.Spec{Name: name}
		return smversion.GetSecretWithVersion(ctx, r.SMClient, spec)
	})

	first := true

	// Process SSM entries in sorted order
	for _, name := range maputil.SortedKeys(ssmEntries) {
		entry := ssmEntries[name]
		result := ssmResults[name]

		if result.Err != nil {
			// Handle fetch error based on operation type
			switch entry.Operation {
			case staging.OperationDelete:
				// Item doesn't exist in AWS anymore - deletion already applied
				if err := r.Store.Unstage(staging.ServiceSSM, name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}
				output.Warning(r.Stderr, "unstaged %s: already deleted in AWS", name)
				continue

			case staging.OperationCreate:
				// Item doesn't exist in AWS - this is expected for create operations
				if !first {
					_, _ = fmt.Fprintln(r.Stdout)
				}
				first = false
				if err := r.outputSSMDiffCreate(opts, name, entry); err != nil {
					return err
				}
				continue

			case staging.OperationUpdate:
				// Item doesn't exist in AWS anymore - staged update is invalid
				if err := r.Store.Unstage(staging.ServiceSSM, name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}
				output.Warning(r.Stderr, "unstaged %s: item no longer exists in AWS", name)
				continue
			}
		}

		if !first {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		first = false

		if err := r.outputSSMDiff(opts, name, entry, result.Value); err != nil {
			return err
		}
	}

	// Process SM entries in sorted order
	for _, name := range maputil.SortedKeys(smEntries) {
		entry := smEntries[name]
		result := smResults[name]

		if result.Err != nil {
			// Handle fetch error based on operation type
			switch entry.Operation {
			case staging.OperationDelete:
				// Item doesn't exist in AWS anymore - deletion already applied
				if err := r.Store.Unstage(staging.ServiceSM, name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}
				output.Warning(r.Stderr, "unstaged %s: already deleted in AWS", name)
				continue

			case staging.OperationCreate:
				// Item doesn't exist in AWS - this is expected for create operations
				if !first {
					_, _ = fmt.Fprintln(r.Stdout)
				}
				first = false
				if err := r.outputSMDiffCreate(opts, name, entry); err != nil {
					return err
				}
				continue

			case staging.OperationUpdate:
				// Item doesn't exist in AWS anymore - staged update is invalid
				if err := r.Store.Unstage(staging.ServiceSM, name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}
				output.Warning(r.Stderr, "unstaged %s: item no longer exists in AWS", name)
				continue
			}
		}

		if !first {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		first = false

		if err := r.outputSMDiff(opts, name, entry, result.Value); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) outputSSMDiff(opts Options, name string, entry staging.Entry, param *types.ParameterHistory) error {
	awsValue := lo.FromPtr(param.Value)
	stagedValue := entry.Value

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.JSONFormat {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, name)
	}

	if awsValue == stagedValue {
		// Auto-unstage since there's no difference
		if err := r.Store.Unstage(staging.ServiceSSM, name); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}
		output.Warning(r.Stderr, "unstaged %s: identical to AWS current", name)
		return nil
	}

	label1 := fmt.Sprintf("%s#%d (AWS)", name, param.Version)
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputSMDiff(opts Options, name string, entry staging.Entry, secret *secretsmanager.GetSecretValueOutput) error {
	awsValue := lo.FromPtr(secret.SecretString)
	stagedValue := entry.Value

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.JSONFormat {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, name)
	}

	if awsValue == stagedValue {
		// Auto-unstage since there's no difference
		if err := r.Store.Unstage(staging.ServiceSM, name); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}
		output.Warning(r.Stderr, "unstaged %s: identical to AWS current", name)
		return nil
	}

	versionID := smversion.TruncateVersionID(lo.FromPtr(secret.VersionId))
	label1 := fmt.Sprintf("%s#%s (AWS)", name, versionID)
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputSSMDiffCreate(opts Options, name string, entry staging.Entry) error {
	stagedValue := entry.Value

	// Format as JSON if enabled
	if opts.JSONFormat {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	label1 := fmt.Sprintf("%s (not in AWS)", name)
	label2 := fmt.Sprintf("%s (staged for creation)", name)

	diff := output.Diff(label1, label2, "", stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputSMDiffCreate(opts Options, name string, entry staging.Entry) error {
	stagedValue := entry.Value

	// Format as JSON if enabled
	if opts.JSONFormat {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	label1 := fmt.Sprintf("%s (not in AWS)", name)
	label2 := fmt.Sprintf("%s (staged for creation)", name)

	diff := output.Diff(label1, label2, "", stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputMetadata(entry staging.Entry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", colors.FieldLabel("Description:"), desc)
	}
	if len(entry.Tags) > 0 {
		var tagPairs []string
		for _, k := range maputil.SortedKeys(entry.Tags) {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, entry.Tags[k]))
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", colors.FieldLabel("Tags:"), strings.Join(tagPairs, ", "))
	}
}
