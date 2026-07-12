// Package diff provides the global diff command for viewing staged changes.
package diff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/samber/lo/it"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
)

// ServiceStrategy pairs a service with its (possibly nil) diff strategy. The
// strategy is nil when no changes are staged for that service and thus no
// provider client was initialized.
type ServiceStrategy struct {
	Service staging.Service
	// Store is this service's own working store (App Configuration and Key Vault
	// live in separate buckets).
	Store store.ReadWriteOperator
	// Strategy is the default (namespace-agnostic) diff strategy.
	Strategy staging.DiffStrategy
	// StrategyFor, when set, resolves a strategy per namespace so each App
	// Configuration entry is diffed under its own namespace. Nil elsewhere.
	StrategyFor func(namespace string) (staging.DiffStrategy, error)
	// Entries/Tags are this service's staged changes, pre-listed from its store,
	// keyed by the (name, namespace) EntryKey.
	Entries map[staging.EntryKey]staging.Entry
	Tags    map[staging.EntryKey]staging.TagEntry
}

// strategyForNamespace returns the strategy scoped to the given namespace, or the
// default strategy when no per-namespace resolver is configured.
func (s ServiceStrategy) strategyForNamespace(namespace string) (staging.DiffStrategy, error) {
	if s.StrategyFor == nil {
		return s.Strategy, nil
	}

	return s.StrategyFor(namespace)
}

// Runner executes the diff command.
type Runner struct {
	// Services lists the configured provider services (with staged changes) in
	// stable display order, each carrying its own store.
	Services []ServiceStrategy
	// ProviderLabel is the human-readable provider name (e.g. "AWS").
	ProviderLabel string
	Stdout        io.Writer
	Stderr        io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	ParseJSON bool
	NoPager   bool
}

// Command returns the diff command for the given provider config.
func Command(cfg stgcli.GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:  "diff",
		Usage: "Show diff of all staged changes",
		Description: `Compare all staged changes against the provider's current values.

For comparing specific versions, use the per-service diff commands.

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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 0 {
				return fmt.Errorf("usage: suve stage diff (no arguments)")
			}

			return runAction(ctx, cmd, cfg)
		},
	}
}

// workingStoreResolver resolves a service's staging store and scope. It matches
// stgcli.WorkingStore; tests substitute a fake to exercise skip-unconfigured and
// store-error paths without touching disk.
type workingStoreResolver func(
	ctx context.Context, resolver staging.ScopeResolver,
) (store.ReadWriteOperator, staging.ResolvedScope, error)

// gatherServices lists each CONFIGURED service's staged changes from its OWN
// store, skipping any service whose scope is not configured (it can hold no
// staged state). It returns only services that actually have staged changes.
// resolve is injected so tests can drive skip-unconfigured and store-error
// propagation.
func gatherServices(
	ctx context.Context, cfg stgcli.GlobalConfig, resolve workingStoreResolver,
) ([]ServiceStrategy, error) {
	services := make([]ServiceStrategy, 0, len(cfg.Services))

	for _, spec := range cfg.Services {
		st, _, err := resolve(ctx, spec.ScopeResolver)
		if errors.Is(err, staging.ErrServiceNotConfigured) {
			continue
		}

		if err != nil {
			return nil, err
		}

		entries, err := st.ListEntries(ctx, spec.Service)
		if err != nil {
			return nil, err
		}

		tags, err := st.ListTags(ctx, spec.Service)
		if err != nil {
			return nil, err
		}

		svcEntries := entries[spec.Service]
		svcTags := tags[spec.Service]

		if len(svcEntries) == 0 && len(svcTags) == 0 {
			continue
		}

		strategy, err := spec.Factory(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize %s client: %w", spec.ParserFactory().ServiceName(), err)
		}

		services = append(services, ServiceStrategy{
			Service:     spec.Service,
			Store:       st,
			Strategy:    strategy,
			StrategyFor: diffStrategyFor(ctx, spec),
			Entries:     svcEntries,
			Tags:        svcTags,
		})
	}

	return services, nil
}

func runAction(ctx context.Context, cmd *cli.Command, cfg stgcli.GlobalConfig) error {
	opts := Options{
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
	}

	resolve := func(
		ctx context.Context, resolver staging.ScopeResolver,
	) (store.ReadWriteOperator, staging.ResolvedScope, error) {
		return stgcli.WorkingStore(ctx, resolver)
	}

	services, err := gatherServices(ctx, cfg, resolve)
	if err != nil {
		return err
	}

	if len(services) == 0 {
		output.Warning(cmd.Root().ErrWriter, "nothing staged")

		return nil
	}

	r := &Runner{
		Services:      services,
		ProviderLabel: cfg.ProviderLabel,
		Stderr:        cmd.Root().ErrWriter,
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r.Stdout = w

		return r.Run(ctx, opts)
	})
}

// diffStrategyFor adapts a spec's StrategyForNamespace to the per-entry resolver
// used during diff, or nil when the service has no namespace axis.
func diffStrategyFor(ctx context.Context, spec stgcli.GlobalServiceSpec) func(string) (staging.DiffStrategy, error) {
	if spec.StrategyForNamespace == nil {
		return nil
	}

	return func(namespace string) (staging.DiffStrategy, error) {
		return spec.StrategyForNamespace(ctx, namespace)
	}
}

// Run executes the diff command over the pre-gathered per-service stores.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	first := true

	// Process value entries in service order (all entries before any tags).
	for _, svc := range r.Services {
		if err := r.diffEntries(ctx, opts, svc, &first); err != nil {
			return err
		}
	}

	// Process tag entries in service order.
	for _, svc := range r.Services {
		for _, key := range staging.SortedEntryKeys(svc.Tags) {
			tagEntry := svc.Tags[key]

			if !first {
				output.Println(r.Stdout, "")
			}

			first = false

			r.outputTagDiff(ctx, svc, key, tagEntry)
		}
	}

	return nil
}

// diffEntries processes one service's staged value entries in sorted order. Each
// entry is keyed by the (name, namespace) EntryKey; it is fetched/unstaged under
// its own namespace via the per-namespace strategy and this service's store.
func (r *Runner) diffEntries(ctx context.Context, opts Options, svc ServiceStrategy, first *bool) error {
	// Fetch all current values in parallel, each under its entry's namespace.
	results := parallel.ExecuteMap(
		ctx,
		svc.Entries,
		func(ctx context.Context, key staging.EntryKey, _ staging.Entry) (*staging.FetchResult, error) {
			strategy, err := svc.strategyForNamespace(key.Namespace)
			if err != nil {
				return nil, err
			}

			return strategy.FetchCurrent(ctx, key.Name)
		},
	)

	for _, key := range staging.SortedEntryKeys(svc.Entries) {
		entry := svc.Entries[key]
		result := results[key]

		if result.Err != nil {
			// Only a genuine "not found" justifies auto-unstaging a staged
			// delete or update. Any other fetch error (expired credentials,
			// throttling, a network blip) must NOT discard staged work on a
			// read-only `stage diff`: surface it and leave the entry staged.
			notFound := errors.Is(result.Err, provider.ErrNotFound)

			switch entry.Operation {
			case staging.OperationDelete:
				if notFound {
					// Item doesn't exist remotely anymore - deletion already applied.
					if err := svc.Store.UnstageEntry(ctx, svc.Service, key); err != nil {
						return fmt.Errorf("failed to unstage %s: %w", key.Name, err)
					}

					output.Warning(r.Stderr, "unstaged %s: already deleted in %s", key.Name, r.ProviderLabel)

					continue
				}

				output.Warning(r.Stderr, "could not diff %s (kept staged): %v", key.Name, result.Err)

				continue

			case staging.OperationCreate:
				// Item doesn't exist remotely - this is expected for create operations
				if !*first {
					output.Println(r.Stdout, "")
				}

				*first = false

				if err := r.outputDiffCreate(opts, key, entry); err != nil {
					return err
				}

				continue

			case staging.OperationUpdate:
				if notFound {
					// Item doesn't exist remotely anymore - staged update is invalid.
					if err := svc.Store.UnstageEntry(ctx, svc.Service, key); err != nil {
						return fmt.Errorf("failed to unstage %s: %w", key.Name, err)
					}

					output.Warning(r.Stderr, "unstaged %s: item no longer exists in %s", key.Name, r.ProviderLabel)

					continue
				}

				output.Warning(r.Stderr, "could not diff %s (kept staged): %v", key.Name, result.Err)

				continue
			}
		}

		if !*first {
			output.Println(r.Stdout, "")
		}

		*first = false

		if err := r.outputDiff(ctx, opts, svc, key, entry, result.Value); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) outputDiff(
	ctx context.Context,
	opts Options,
	svc ServiceStrategy,
	key staging.EntryKey,
	entry staging.Entry,
	fr *staging.FetchResult,
) error {
	name := key.Name
	remoteValue := fr.Value
	stagedValue := lo.FromPtr(entry.Value)

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// The auto-unstage decision is made on the RAW values, never on the
	// --parse-json-normalized ones: whether staged work survives must not depend
	// on a display flag, and it must match the service-level DiffUseCase which
	// compares raw values. It also never applies to a delete (deleting is not a
	// no-op just because the current value is the empty string).
	if entry.Operation != staging.OperationDelete && remoteValue == stagedValue {
		if err := svc.Store.UnstageEntry(ctx, svc.Service, key); err != nil {
			return fmt.Errorf("failed to unstage %s: %w", name, err)
		}

		output.Warning(r.Stderr, "unstaged %s: identical to %s current", name, r.ProviderLabel)

		return nil
	}

	// Format for rendering only.
	displayRemote, displayStaged := remoteValue, stagedValue
	if opts.ParseJSON {
		displayRemote, displayStaged = jsonutil.TryFormatOrWarn2(remoteValue, stagedValue, r.Stderr, name)
	}

	disp := diffDisplayName(key)
	label1 := fmt.Sprintf("%s%s (%s)", disp, fr.Identifier, r.ProviderLabel)
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), disp)

	diff := output.Diff(r.Stdout, label1, label2, displayRemote, displayStaged)

	// Raw values differ but --parse-json renders no textual diff: the staged
	// update only reformats JSON. It remains staged (decided on raw above).
	if diff == "" {
		output.Warning(r.Stderr, "%s: staged value differs from %s only in JSON formatting", name, r.ProviderLabel)

		return nil
	}

	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

func (r *Runner) outputDiffCreate(opts Options, key staging.EntryKey, entry staging.Entry) error {
	stagedValue := lo.FromPtr(entry.Value)

	// Format as JSON if enabled
	if opts.ParseJSON {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	disp := diffDisplayName(key)
	label1 := fmt.Sprintf("%s (not in %s)", disp, r.ProviderLabel)
	label2 := fmt.Sprintf("%s (staged for creation)", disp)

	diff := output.Diff(r.Stdout, label1, label2, "", stagedValue)
	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)

	return nil
}

// diffDisplayName renders an entry key for display, appending the App
// Configuration namespace when present (empty is the null/default namespace and
// every other provider, shown bare).
func diffDisplayName(key staging.EntryKey) string {
	if key.Namespace == "" {
		return key.Name
	}

	return fmt.Sprintf("%s [%s]", key.Name, key.Namespace)
}

func (r *Runner) outputMetadata(entry staging.Entry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		output.Printf(r.Stdout, "%s %s\n", colors.For(r.Stdout).FieldLabel("Description:"), desc)
	}
}

func (r *Runner) outputTagDiff(ctx context.Context, svc ServiceStrategy, key staging.EntryKey, tagEntry staging.TagEntry) {
	output.Printf(r.Stdout, "%s %s (staged tag changes)\n", colors.For(r.Stdout).Info("Tags:"), diffDisplayName(key))

	if len(tagEntry.Add) > 0 {
		tagPairs := slices.Collect(it.Map(maputil.SortedKeys(tagEntry.Add), func(k string) string {
			return fmt.Sprintf("%s=%s", k, tagEntry.Add[k])
		}))

		output.Printf(r.Stdout, "  %s %s\n", colors.For(r.Stdout).OpAdd("+"), strings.Join(tagPairs, ", "))
	}

	if tagEntry.Remove.Len() > 0 {
		// Fetch current tag values under this entry's namespace. A resolver error
		// or a fetch error both yield a nil map (keys are then shown without values).
		var currentTags map[string]string
		if strategy, err := svc.strategyForNamespace(key.Namespace); err == nil && strategy != nil {
			currentTags, _ = strategy.FetchCurrentTags(ctx, key.Name)
		}

		r.outputRemovedTags(tagEntry.Remove, currentTags)
	}
}

func (r *Runner) outputRemovedTags(remove maputil.Set[string], currentTags map[string]string) {
	tagPairs := slices.Collect(it.Map(maputil.SortedKeys(remove), func(k string) string {
		if currentTags != nil {
			if v := currentTags[k]; v != "" {
				return fmt.Sprintf("%s=%s", k, v)
			}
		}

		return k
	}))

	output.Printf(r.Stdout, "  %s %s\n", colors.For(r.Stdout).OpDelete("-"), strings.Join(tagPairs, ", "))
}
