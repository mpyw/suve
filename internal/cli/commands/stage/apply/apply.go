// Package apply provides the global apply command for applying all staged changes.
package apply

import (
	"context"
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
	Service  staging.Service
	Strategy staging.ApplyStrategy
}

// serviceConflictCheck holds entries and strategy for a single service's conflict checking.
type serviceConflictCheck struct {
	entries  map[staging.EntryKey]staging.Entry
	strategy staging.ApplyStrategy
}

// Runner executes the apply command.
type Runner struct {
	// Services lists the provider services in stable apply order.
	Services []ServiceApply
	// ProviderLabel is the human-readable provider name (e.g. "AWS").
	ProviderLabel   string
	Store           store.ReadWriteOperator
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
	store, resolved, err := stgcli.WorkingStore(ctx, cfg.ScopeResolver)
	if err != nil {
		return err
	}

	allStaged, err := store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	allTagStaged, err := store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	// Count staged changes across all services.
	totalStaged := 0
	for _, svc := range cfg.Services {
		totalStaged += len(allStaged[svc.Service]) + len(allTagStaged[svc.Service])
	}

	if totalStaged == 0 {
		output.Info(cmd.Root().Writer, "No changes staged.")

		return nil
	}

	// Confirm apply.
	prompter := &confirm.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
		Target: resolved.Target,
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
		ProviderLabel:   cfg.ProviderLabel,
		Store:           store,
		Stdout:          cmd.Root().Writer,
		Stderr:          cmd.Root().ErrWriter,
		IgnoreConflicts: cmd.Bool("ignore-conflicts"),
	}

	// Initialize a strategy per service that has staged changes.
	for _, svc := range cfg.Services {
		has := len(allStaged[svc.Service]) > 0 || len(allTagStaged[svc.Service]) > 0

		var strategy staging.ApplyStrategy

		if has {
			full, err := svc.Factory(ctx)
			if err != nil {
				return err
			}

			strategy = full
		}

		r.Services = append(r.Services, ServiceApply{Service: svc.Service, Strategy: strategy})
	}

	return r.Run(ctx)
}

// Run executes the apply command.
func (r *Runner) Run(ctx context.Context) error {
	allStaged, err := r.Store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	allTagStaged, err := r.Store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	// Check for conflicts unless --ignore-conflicts is specified.
	if !r.IgnoreConflicts {
		var checks []serviceConflictCheck

		for _, svc := range r.Services {
			entries := allStaged[svc.Service]
			if len(entries) > 0 && svc.Strategy != nil {
				checks = append(checks, serviceConflictCheck{entries: entries, strategy: svc.Strategy})
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
		staged := allStaged[svc.Service]
		if len(staged) == 0 {
			continue
		}

		output.Info(r.Stdout, "Applying %s...", svc.Strategy.ServiceName())
		succeeded, failed := r.applyService(ctx, svc.Strategy, staged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Apply tag changes in service order.
	for _, svc := range r.Services {
		staged := allTagStaged[svc.Service]
		if len(staged) == 0 {
			continue
		}

		output.Info(r.Stdout, "Applying %s tags...", svc.Strategy.ServiceName())
		succeeded, failed := r.applyTagService(ctx, svc.Strategy, staged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Summary
	if totalFailed > 0 {
		return fmt.Errorf("applied %d, failed %d", totalSucceeded, totalFailed)
	}

	return nil
}

func (r *Runner) applyService(
	ctx context.Context,
	strategy staging.ApplyStrategy,
	staged map[staging.EntryKey]staging.Entry,
) (succeeded, failed int) {
	service := strategy.Service()
	serviceName := strategy.ServiceName()

	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, key staging.EntryKey, entry staging.Entry) (staging.Operation, error) {
		err := strategy.Apply(ctx, key.Name, entry)

		return entry.Operation, err
	})

	for _, key := range staging.SortedEntryKeys(staged) {
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

			if err := r.Store.UnstageEntry(ctx, service, key); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", key.Name, err)
			}

			succeeded++
		}
	}

	return succeeded, failed
}

func (r *Runner) applyTagService(
	ctx context.Context,
	strategy staging.ApplyStrategy,
	staged map[staging.EntryKey]staging.TagEntry,
) (succeeded, failed int) {
	service := strategy.Service()
	serviceName := strategy.ServiceName()

	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, key staging.EntryKey, tagEntry staging.TagEntry) (struct{}, error) {
		err := strategy.ApplyTags(ctx, key.Name, tagEntry)

		return struct{}{}, err
	})

	for _, key := range staging.SortedEntryKeys(staged) {
		tagEntry := staged[key]

		result := results[key]
		if result.Err != nil {
			output.Failed(r.Stderr, serviceName+": "+key.Name+" (tags)", result.Err)

			failed++
		} else {
			output.Success(r.Stdout, "%s: Tagged %s%s", serviceName, key.Name, formatTagApplySummary(tagEntry))

			if err := r.Store.UnstageTag(ctx, service, key); err != nil {
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

// checkAllConflicts checks all services for conflicts and returns a combined map of conflicting names.
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
