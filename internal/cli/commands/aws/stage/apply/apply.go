// Package apply provides the global apply command for applying all staged changes.
package apply

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
)

// ServiceApply pairs a service with its (possibly nil) apply strategy. The
// strategy is nil when no changes are staged for that service.
type ServiceApply struct {
	Service staging.Service
	// Store is this service's own working store (App Configuration and Key Vault
	// live in separate buckets).
	Store store.ReadWriteOperator
	// Strategy is the default (namespace-agnostic) apply strategy.
	Strategy staging.ApplyStrategy
	// StrategyFor, when set, resolves a strategy per namespace so each App
	// Configuration entry applies under its own namespace. Nil elsewhere.
	StrategyFor func(namespace string) (staging.ApplyStrategy, error)
	// Entries/Tags are this service's staged changes, pre-listed from its store,
	// keyed by the (name, namespace) EntryKey.
	Entries map[staging.EntryKey]staging.Entry
	Tags    map[staging.EntryKey]staging.TagEntry
}

// strategyForNamespace returns the strategy scoped to the given namespace, or the
// default strategy when no per-namespace resolver is configured.
func (s ServiceApply) strategyForNamespace(namespace string) (staging.ApplyStrategy, error) {
	if s.StrategyFor == nil {
		return s.Strategy, nil
	}

	return s.StrategyFor(namespace)
}

// serviceConflictCheck holds the staged entries and tags plus the per-namespace
// strategy resolver for a single service's conflict checking. resolve mirrors the
// apply path so each entry/tag is probed against its own namespace's remote state.
type serviceConflictCheck struct {
	serviceName string
	entries     map[staging.EntryKey]staging.Entry
	tags        map[staging.EntryKey]staging.TagEntry
	resolve     staging.ApplyStrategyResolver
}

// conflictItem identifies a conflicting staged entry, qualified by its service.
// Conflicts are tracked per (service, key) rather than per key alone so two
// services staging the same (name, namespace) — e.g. an SSM param and a Secrets
// Manager secret both named "foo" — are counted and reported separately instead
// of collapsing into one.
type conflictItem struct {
	serviceName string
	key         staging.EntryKey
}

// Runner executes the apply command.
type Runner struct {
	// Services lists the configured provider services (with staged changes) in
	// stable apply order, each carrying its own store.
	Services []ServiceApply
	// ProviderLabel is the human-readable provider name (e.g. "AWS").
	ProviderLabel   string
	Stdout          io.Writer
	Stderr          io.Writer
	IgnoreConflicts bool
}

// Command returns the global apply command for the given provider config.
func Command(cfg stgcli.GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Aliases: []string{"push"},
		Usage:   "Apply all staged changes",
		Description: `Apply all staged changes for the active provider's services.

After successful apply, the staged changes are cleared.

Use 'suve stage status' to view all staged changes before applying.

CONFLICT DETECTION:
   Before applying, suve checks for conflicts to prevent lost updates:
   - For new resources: checks if someone else created it after staging
   - For existing resources: checks if it was modified after staging
   Use --ignore-conflicts to force apply despite conflicts.

EXAMPLES:
   suve stage apply                      Apply all staged changes (with confirmation)
   suve stage apply --yes                Apply without confirmation
   suve stage apply --ignore-conflicts   Apply even if conflicts detected`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "ignore-conflicts",
				Usage: "Apply even if the remote store was modified after staging",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
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
// staged state). It returns the services that actually have staged changes,
// their confirmation targets, and the total staged count. resolve is injected so
// tests can drive skip-unconfigured and store-error propagation.
func gatherServices(
	ctx context.Context, cfg stgcli.GlobalConfig, resolve workingStoreResolver,
) (svcs []ServiceApply, targets []string, totalStaged int, err error) {
	for _, spec := range cfg.Services {
		st, resolved, err := resolve(ctx, spec.ScopeResolver)
		if errors.Is(err, staging.ErrServiceNotConfigured) {
			continue
		}

		if err != nil {
			return nil, nil, 0, err
		}

		entries, err := st.ListEntries(ctx, spec.Service)
		if err != nil {
			return nil, nil, 0, err
		}

		tags, err := st.ListTags(ctx, spec.Service)
		if err != nil {
			return nil, nil, 0, err
		}

		svcEntries := entries[spec.Service]
		svcTags := tags[spec.Service]

		totalStaged += len(svcEntries) + len(svcTags)
		if len(svcEntries) == 0 && len(svcTags) == 0 {
			continue
		}

		strategy, err := spec.Factory(ctx)
		if err != nil {
			return nil, nil, 0, err
		}

		targets = append(targets, resolved.Target)
		svcs = append(svcs, ServiceApply{
			Service:     spec.Service,
			Store:       st,
			Strategy:    strategy,
			StrategyFor: applyStrategyFor(ctx, spec),
			Entries:     svcEntries,
			Tags:        svcTags,
		})
	}

	return svcs, targets, totalStaged, nil
}

func runAction(ctx context.Context, cmd *cli.Command, cfg stgcli.GlobalConfig) error {
	resolve := func(
		ctx context.Context, resolver staging.ScopeResolver,
	) (store.ReadWriteOperator, staging.ResolvedScope, error) {
		return stgcli.WorkingStore(ctx, resolver)
	}

	svcs, targets, totalStaged, err := gatherServices(ctx, cfg, resolve)
	if err != nil {
		return err
	}

	if totalStaged == 0 {
		output.Info(cmd.Root().Writer, "No changes staged.")

		return nil
	}

	// Confirm apply (once, across all services).
	prompter := &confirm.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
		Target: strings.Join(targets, ", "),
	}

	message := fmt.Sprintf("Apply %d staged change(s) to %s?", totalStaged, cfg.ProviderLabel)

	confirmed, err := prompter.Confirm(message, cmd.Bool("yes"))
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &Runner{
		Services:        svcs,
		ProviderLabel:   cfg.ProviderLabel,
		Stdout:          cmd.Root().Writer,
		Stderr:          cmd.Root().ErrWriter,
		IgnoreConflicts: cmd.Bool("ignore-conflicts"),
	}

	return r.Run(ctx)
}

// applyStrategyFor adapts a spec's StrategyForNamespace to the per-entry resolver
// used during apply, or nil when the service has no namespace axis.
func applyStrategyFor(ctx context.Context, spec stgcli.GlobalServiceSpec) func(string) (staging.ApplyStrategy, error) {
	if spec.StrategyForNamespace == nil {
		return nil
	}

	return func(namespace string) (staging.ApplyStrategy, error) {
		return spec.StrategyForNamespace(ctx, namespace)
	}
}

// Run executes the apply command over the pre-gathered per-service stores.
func (r *Runner) Run(ctx context.Context) error {
	// Check for conflicts unless --ignore-conflicts is specified.
	if !r.IgnoreConflicts {
		var checks []serviceConflictCheck

		for _, svc := range r.Services {
			// Include a service if it has staged entries OR staged tags, so a
			// tags-only service is still conflict-checked before apply.
			if len(svc.Entries) == 0 && len(svc.Tags) == 0 {
				continue
			}

			checks = append(checks, serviceConflictCheck{
				serviceName: svc.Strategy.ServiceName(),
				entries:     svc.Entries,
				tags:        svc.Tags,
				resolve:     svc.strategyForNamespace,
			})
		}

		allConflicts := r.checkAllConflicts(ctx, checks)
		if len(allConflicts) > 0 {
			for _, c := range allConflicts {
				output.Warning(r.Stderr, "conflict detected for %s (%s): %s was modified after staging",
					c.key.Label(), c.serviceName, r.ProviderLabel)
			}

			return fmt.Errorf("apply rejected: %d conflict(s) detected (use --ignore-conflicts to force)", len(allConflicts))
		}
	}

	var totalSucceeded, totalFailed int

	// Apply value changes in service order.
	for _, svc := range r.Services {
		if len(svc.Entries) == 0 {
			continue
		}

		output.Info(r.Stdout, "Applying %s...", svc.Strategy.ServiceName())
		succeeded, failed := r.applyService(ctx, svc)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Apply tag changes in service order.
	for _, svc := range r.Services {
		if len(svc.Tags) == 0 {
			continue
		}

		output.Info(r.Stdout, "Applying %s tags...", svc.Strategy.ServiceName())
		succeeded, failed := r.applyTagService(ctx, svc)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Summary
	if totalFailed > 0 {
		return fmt.Errorf("applied %d, failed %d", totalSucceeded, totalFailed)
	}

	return nil
}

func (r *Runner) applyService(ctx context.Context, svc ServiceApply) (succeeded, failed int) {
	serviceName := svc.Strategy.ServiceName()

	// Entries are keyed by the (name, namespace) EntryKey; apply each through the
	// strategy scoped to its own namespace and unstage under that key.
	results := parallel.ExecuteMap(ctx, svc.Entries, func(ctx context.Context, key staging.EntryKey, entry staging.Entry) (staging.Operation, error) {
		strategy, err := svc.strategyForNamespace(key.Namespace)
		if err != nil {
			return entry.Operation, err
		}

		return entry.Operation, strategy.Apply(ctx, key.Name, entry)
	})

	for _, key := range staging.SortedEntryKeys(svc.Entries) {
		result := results[key]
		if result.Err != nil {
			output.Failed(r.Stderr, serviceName+": "+key.Name, result.Err)

			failed++
		} else {
			switch result.Value {
			case staging.OperationCreate:
				output.Success(r.Stdout, "%s: Created %s", serviceName, key.Name)
			case staging.OperationUpdate:
				output.Success(r.Stdout, "%s: Updated %s", serviceName, key.Name)
			case staging.OperationDelete:
				output.Success(r.Stdout, "%s: Deleted %s", serviceName, key.Name)
			}

			if err := svc.Store.UnstageEntry(ctx, svc.Service, key); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", key.Name, err)
			}

			succeeded++
		}
	}

	return succeeded, failed
}

func (r *Runner) applyTagService(ctx context.Context, svc ServiceApply) (succeeded, failed int) {
	serviceName := svc.Strategy.ServiceName()

	// Tags share the (name, namespace) EntryKey; resolve the strategy per
	// namespace so App Configuration tags target the right store partition.
	results := parallel.ExecuteMap(ctx, svc.Tags, func(ctx context.Context, key staging.EntryKey, tagEntry staging.TagEntry) (struct{}, error) {
		strategy, err := svc.strategyForNamespace(key.Namespace)
		if err != nil {
			return struct{}{}, err
		}

		return struct{}{}, strategy.ApplyTags(ctx, key.Name, tagEntry)
	})

	for _, key := range staging.SortedEntryKeys(svc.Tags) {
		tagEntry := svc.Tags[key]

		result := results[key]
		if result.Err != nil {
			output.Failed(r.Stderr, serviceName+": "+key.Name+" (tags)", result.Err)

			failed++
		} else {
			output.Success(r.Stdout, "%s: Tagged %s%s", serviceName, key.Name, formatTagApplySummary(tagEntry))

			if err := svc.Store.UnstageTag(ctx, svc.Service, key); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s tags: %v", key.Name, err)
			}

			succeeded++
		}
	}

	return succeeded, failed
}

func formatTagApplySummary(tagEntry staging.TagEntry) string {
	var parts []string
	if len(tagEntry.Add) > 0 {
		parts = append(parts, fmt.Sprintf("+%d", len(tagEntry.Add)))
	}

	if tagEntry.Remove.Len() > 0 {
		parts = append(parts, fmt.Sprintf("-%d", tagEntry.Remove.Len()))
	}

	if len(parts) == 0 {
		return ""
	}

	return " [" + strings.Join(parts, ", ") + "]"
}

// checkAllConflicts checks all services for both value and tag conflicts and
// returns them qualified by service, in a stable (service order, then sorted key)
// order. Two services with a conflict on the same (name, namespace) yield two
// distinct items; within one service a key that conflicts on both its value and
// its tags is reported once.
func (r *Runner) checkAllConflicts(ctx context.Context, checks []serviceConflictCheck) []conflictItem {
	all := make([]conflictItem, 0, len(checks))

	for _, check := range checks {
		merged := make(map[staging.EntryKey]struct{})

		maps.Copy(merged, staging.CheckConflicts(ctx, check.resolve, check.entries))
		maps.Copy(merged, staging.CheckTagConflicts(ctx, check.resolve, check.tags))

		for _, key := range staging.SortedEntryKeys(merged) {
			all = append(all, conflictItem{serviceName: check.serviceName, key: key})
		}
	}

	return all
}
