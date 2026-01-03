// Package diff provides the global diff command for viewing staged changes.
package diff

import (
	"context"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/version/paramversion"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// ParamClient is the interface for SSM Parameter Store operations.
type ParamClient interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
}

// SecretClient is the interface for Secrets Manager operations.
type SecretClient interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIdsAPI
}

// Runner executes the diff command.
type Runner struct {
	ParamClient  ParamClient
	SecretClient SecretClient
	Store        *staging.Store
	Stdout       io.Writer
	Stderr       io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	ParseJSON bool
	NoPager   bool
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "diff",
		Usage: "Show diff of all staged changes (SSM Parameter Store and Secrets Manager)",
		Description: `Compare all staged changes against AWS current values.

For comparing specific versions, use 'suve param diff' or 'suve secret diff'.

EXAMPLES:
   suve stage diff     Show diff of all staged changes
   suve stage diff -j  Show diff with JSON formatting`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
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
	paramStaged, err := store.List(staging.ServiceParam)
	if err != nil {
		return err
	}
	secretStaged, err := store.List(staging.ServiceSecret)
	if err != nil {
		return err
	}

	hasParam := len(paramStaged[staging.ServiceParam]) > 0
	hasSecret := len(secretStaged[staging.ServiceSecret]) > 0

	if !hasParam && !hasSecret {
		output.Warning(cmd.Root().ErrWriter, "nothing staged")
		return nil
	}

	r := &Runner{
		Store:  store,
		Stderr: cmd.Root().ErrWriter,
	}

	// Initialize clients only if needed
	if hasParam {
		paramClient, err := infra.NewParamClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize SSM Parameter Store client: %w", err)
		}
		r.ParamClient = paramClient
	}

	if hasSecret {
		secretClient, err := infra.NewSecretClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Secrets Manager client: %w", err)
		}
		r.SecretClient = secretClient
	}

	opts := Options{
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
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

	paramEntries := allEntries[staging.ServiceParam]
	secretEntries := allEntries[staging.ServiceSecret]

	// Fetch all values in parallel
	paramResults := parallel.ExecuteMap(ctx, paramEntries, func(ctx context.Context, name string, _ staging.Entry) (*paramapi.ParameterHistory, error) {
		spec := &paramversion.Spec{Name: name}
		return paramversion.GetParameterWithVersion(ctx, r.ParamClient, spec)
	})

	secretResults := parallel.ExecuteMap(ctx, secretEntries, func(ctx context.Context, name string, _ staging.Entry) (*secretapi.GetSecretValueOutput, error) {
		spec := &secretversion.Spec{Name: name}
		return secretversion.GetSecretWithVersion(ctx, r.SecretClient, spec)
	})

	first := true

	// Process SSM Parameter Store entries in sorted order
	for _, name := range maputil.SortedKeys(paramEntries) {
		entry := paramEntries[name]
		result := paramResults[name]

		if result.Err != nil {
			// Handle fetch error based on operation type
			switch entry.Operation {
			case staging.OperationDelete:
				// Item doesn't exist in AWS anymore - deletion already applied
				if err := r.Store.Unstage(staging.ServiceParam, name); err != nil {
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
				if err := r.outputParamDiffCreate(opts, name, entry); err != nil {
					return err
				}
				continue

			case staging.OperationUpdate:
				// Item doesn't exist in AWS anymore - staged update is invalid
				if err := r.Store.Unstage(staging.ServiceParam, name); err != nil {
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

		if err := r.outputParamDiff(opts, name, entry, result.Value); err != nil {
			return err
		}
	}

	// Process Secrets Manager entries in sorted order
	for _, name := range maputil.SortedKeys(secretEntries) {
		entry := secretEntries[name]
		result := secretResults[name]

		if result.Err != nil {
			// Handle fetch error based on operation type
			switch entry.Operation {
			case staging.OperationDelete:
				// Item doesn't exist in AWS anymore - deletion already applied
				if err := r.Store.Unstage(staging.ServiceSecret, name); err != nil {
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
				if err := r.outputSecretDiffCreate(opts, name, entry); err != nil {
					return err
				}
				continue

			case staging.OperationUpdate:
				// Item doesn't exist in AWS anymore - staged update is invalid
				if err := r.Store.Unstage(staging.ServiceSecret, name); err != nil {
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

		if err := r.outputSecretDiff(opts, name, entry, result.Value); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) outputParamDiff(opts Options, name string, entry staging.Entry, param *paramapi.ParameterHistory) error {
	awsValue := lo.FromPtr(param.Value)
	stagedValue := lo.FromPtr(entry.Value)

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.ParseJSON {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, name)
	}

	if awsValue == stagedValue {
		// Auto-unstage since there's no difference
		if err := r.Store.Unstage(staging.ServiceParam, name); err != nil {
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

func (r *Runner) outputSecretDiff(opts Options, name string, entry staging.Entry, secret *secretapi.GetSecretValueOutput) error {
	awsValue := lo.FromPtr(secret.SecretString)
	stagedValue := lo.FromPtr(entry.Value)

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.ParseJSON {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, name)
	}

	if awsValue == stagedValue {
		// Auto-unstage since there's no difference
		if err := r.Store.Unstage(staging.ServiceSecret, name); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}
		output.Warning(r.Stderr, "unstaged %s: identical to AWS current", name)
		return nil
	}

	versionID := secretversion.TruncateVersionID(lo.FromPtr(secret.VersionId))
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

func (r *Runner) outputParamDiffCreate(opts Options, name string, entry staging.Entry) error {
	stagedValue := lo.FromPtr(entry.Value)

	// Format as JSON if enabled
	if opts.ParseJSON {
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

func (r *Runner) outputSecretDiffCreate(opts Options, name string, entry staging.Entry) error {
	stagedValue := lo.FromPtr(entry.Value)

	// Format as JSON if enabled
	if opts.ParseJSON {
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
}
