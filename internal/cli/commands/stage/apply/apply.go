// Package apply provides the global apply command for applying all staged changes.
package apply

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// serviceConflictCheck holds entries and strategy for a single service's conflict checking.
type serviceConflictCheck struct {
	entries  map[staging.EntryKey]staging.Entry
	strategy staging.ApplyStrategy
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

func runAction(ctx context.Context, cmd *cli.Command, cfg stgcli.GlobalConfig) error {
	var (
		svcs        []ServiceApply
		targets     []string
		totalStaged int
	)

	// Gather each CONFIGURED service's staged changes from its OWN store. A
	// service whose scope is not configured is skipped (it can hold no state).
	for _, spec := range cfg.Services {
		st, resolved, err := stgcli.WorkingStore(ctx, spec.ScopeResolver)
		if errors.Is(err, staging.ErrServiceNotConfigured) {
			continue
		}

		if err != nil {
			return err
		}

		entries, err := st.ListEntries(ctx, spec.Service)
		if err != nil {
			return err
		}

		tags, err := st.ListTags(ctx, spec.Service)
		if err != nil {
			return err
		}

		svcEntries := entries[spec.Service]
		svcTags := tags[spec.Service]

		totalStaged += len(svcEntries) + len(svcTags)
		if len(svcEntries) == 0 && len(svcTags) == 0 {
			continue
		}

		strategy, err := spec.Factory(ctx)
		if err != nil {
			return err
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
			if len(svc.Entries) > 0 {
				checks = append(checks, serviceConflictCheck{entries: svc.Entries, strategy: svc.Strategy})
			}
		}

		allConflicts := r.checkAllConflicts(ctx, checks)
		if len(allConflicts) > 0 {
			for _, key := range staging.SortedEntryKeys(allConflicts) {
				output.Warning(r.Stderr, "conflict detected for %s: %s was modified after staging", key.Name, r.ProviderLabel)
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

// checkAllConflicts checks all services for conflicts and returns a combined map of conflicting keys.
func (r *Runner) checkAllConflicts(ctx context.Context, checks []serviceConflictCheck) map[staging.EntryKey]struct{} {
	allConflicts := make(map[staging.EntryKey]struct{})

	for _, check := range checks {
		conflicts := staging.CheckConflicts(ctx, check.strategy, check.entries)
		for key := range conflicts {
			allConflicts[key] = struct{}{}
		}
	}

	return allConflicts
}
