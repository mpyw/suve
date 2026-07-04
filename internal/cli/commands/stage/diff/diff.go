// Package diff provides the global diff command for viewing staged changes.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// Runner executes the diff command.
type Runner struct {
	ParamStrategy  staging.DiffStrategy
	SecretStrategy staging.DiffStrategy
	Store          store.ReadWriteOperator
	Stdout         io.Writer
	Stderr         io.Writer
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

	identity, err := infra.GetAWSIdentity(ctx)
	if err != nil {
		return fmt.Errorf("failed to get AWS identity: %w", err)
	}

	store, err := file.NewWorkingStore(provider.AWSScope(identity.AccountID, identity.Region))
	if err != nil {
		return fmt.Errorf("failed to create staging store: %w", err)
	}

	opts := Options{
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
	}

	// Check if there are any staged changes before creating clients
	paramStaged, err := store.ListEntries(ctx, staging.ServiceParam)
	if err != nil {
		return err
	}

	secretStaged, err := store.ListEntries(ctx, staging.ServiceSecret)
	if err != nil {
		return err
	}

	paramTagStaged, err := store.ListTags(ctx, staging.ServiceParam)
	if err != nil {
		return err
	}

	secretTagStaged, err := store.ListTags(ctx, staging.ServiceSecret)
	if err != nil {
		return err
	}

	hasParam := len(paramStaged[staging.ServiceParam]) > 0 || len(paramTagStaged[staging.ServiceParam]) > 0
	hasSecret := len(secretStaged[staging.ServiceSecret]) > 0 || len(secretTagStaged[staging.ServiceSecret]) > 0

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

		r.ParamStrategy = staging.NewParamStrategy(paramClient)
	}

	if hasSecret {
		secretClient, err := infra.NewSecretClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize Secrets Manager client: %w", err)
		}

		r.SecretStrategy = staging.NewSecretStrategy(secretClient)
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r.Stdout = w

		return r.Run(ctx, opts)
	})
}

// serviceStrategy pairs a service with its (possibly nil) diff strategy.
// The strategy is nil when no changes are staged for that service and thus no
// AWS client was initialized.
type serviceStrategy struct {
	service  staging.Service
	strategy staging.DiffStrategy
}

// strategies returns the ordered list of services to process.
// The order (SSM Parameter Store, then Secrets Manager) is significant: it
// determines the output ordering of both entry and tag diffs.
func (r *Runner) strategies() []serviceStrategy {
	return []serviceStrategy{
		{staging.ServiceParam, r.ParamStrategy},
		{staging.ServiceSecret, r.SecretStrategy},
	}
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	allEntries, err := r.Store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	allTagEntries, err := r.Store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	first := true

	// Process value entries in service order (all entries before any tags).
	for _, s := range r.strategies() {
		if err := r.diffEntries(ctx, opts, s.strategy, allEntries[s.service], &first); err != nil {
			return err
		}
	}

	// Process tag entries in service order.
	for _, s := range r.strategies() {
		tagEntries := allTagEntries[s.service]
		for _, name := range maputil.SortedKeys(tagEntries) {
			tagEntry := tagEntries[name]

			if !first {
				output.Println(r.Stdout, "")
			}

			first = false

			r.outputTagDiff(ctx, s.strategy, name, tagEntry)
		}
	}

	return nil
}

// diffEntries processes staged value entries for a single service in sorted order.
func (r *Runner) diffEntries(
	ctx context.Context,
	opts Options,
	strategy staging.DiffStrategy,
	entries map[string]staging.Entry,
	first *bool,
) error {
	// Fetch all current values in parallel.
	results := parallel.ExecuteMap(
		ctx,
		entries,
		func(ctx context.Context, name string, _ staging.Entry) (*staging.FetchResult, error) {
			return strategy.FetchCurrent(ctx, name)
		},
	)

	for _, name := range maputil.SortedKeys(entries) {
		entry := entries[name]
		result := results[name]

		if result.Err != nil {
			// Handle fetch error based on operation type
			switch entry.Operation {
			case staging.OperationDelete:
				// Item doesn't exist in AWS anymore - deletion already applied
				if err := r.Store.UnstageEntry(ctx, strategy.Service(), name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}

				output.Warning(r.Stderr, "unstaged %s: already deleted in AWS", name)

				continue

			case staging.OperationCreate:
				// Item doesn't exist in AWS - this is expected for create operations
				if !*first {
					output.Println(r.Stdout, "")
				}

				*first = false

				if err := r.outputDiffCreate(opts, name, entry); err != nil {
					return err
				}

				continue

			case staging.OperationUpdate:
				// Item doesn't exist in AWS anymore - staged update is invalid
				if err := r.Store.UnstageEntry(ctx, strategy.Service(), name); err != nil {
					return fmt.Errorf("failed to unstage %s: %w", name, err)
				}

				output.Warning(r.Stderr, "unstaged %s: item no longer exists in AWS", name)

				continue
			}
		}

		if !*first {
			output.Println(r.Stdout, "")
		}

		*first = false

		if err := r.outputDiff(ctx, opts, strategy, name, entry, result.Value); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) outputDiff(
	ctx context.Context,
	opts Options,
	strategy staging.DiffStrategy,
	name string,
	entry staging.Entry,
	fr *staging.FetchResult,
) error {
	awsValue := fr.Value
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
		if err := r.Store.UnstageEntry(ctx, strategy.Service(), name); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}

		output.Warning(r.Stderr, "unstaged %s: identical to AWS current", name)

		return nil
	}

	label1 := fmt.Sprintf("%s%s (AWS)", name, fr.Identifier)
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputDiffCreate(opts Options, name string, entry staging.Entry) error {
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
	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputMetadata(entry staging.Entry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		output.Printf(r.Stdout, "%s %s\n", colors.FieldLabel("Description:"), desc)
	}
}

func (r *Runner) outputTagDiff(ctx context.Context, strategy staging.DiffStrategy, name string, tagEntry staging.TagEntry) {
	output.Printf(r.Stdout, "%s %s (staged tag changes)\n", colors.Info("Tags:"), name)

	if len(tagEntry.Add) > 0 {
		tagPairs := make([]string, 0, len(tagEntry.Add))
		for _, k := range maputil.SortedKeys(tagEntry.Add) {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, tagEntry.Add[k]))
		}

		output.Printf(r.Stdout, "  %s %s\n", colors.OpAdd("+"), strings.Join(tagPairs, ", "))
	}

	if tagEntry.Remove.Len() > 0 {
		// Fetch current tag values from AWS. A nil strategy (no staged value
		// changes for this service, so no client) or a fetch error both yield a
		// nil map, matching the previous per-service helper behavior.
		var currentTags map[string]string
		if strategy != nil {
			currentTags, _ = strategy.FetchCurrentTags(ctx, name)
		}

		r.outputRemovedTags(tagEntry.Remove, currentTags)
	}
}

func (r *Runner) outputRemovedTags(remove maputil.Set[string], currentTags map[string]string) {
	tagPairs := make([]string, 0, remove.Len())
	for _, k := range maputil.SortedKeys(remove) {
		if currentTags != nil {
			if v := currentTags[k]; v != "" {
				tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, v))
			} else {
				tagPairs = append(tagPairs, k)
			}
		} else {
			tagPairs = append(tagPairs, k)
		}
	}

	output.Printf(r.Stdout, "  %s %s\n", colors.OpDelete("-"), strings.Join(tagPairs, ", "))
}
