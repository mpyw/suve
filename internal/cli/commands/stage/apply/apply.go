// Package apply provides the global apply command for applying all staged changes.
package apply

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// serviceConflictCheck holds entries and strategy for a single service's conflict checking.
type serviceConflictCheck struct {
	entries  map[string]staging.Entry
	strategy staging.ApplyStrategy
}

// Runner executes the apply command.
type Runner struct {
	ParamStrategy   staging.ApplyStrategy
	SecretStrategy  staging.ApplyStrategy
	Store           *staging.Store
	Stdout          io.Writer
	Stderr          io.Writer
	IgnoreConflicts bool
}

// Command returns the global apply command.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Aliases: []string{"push"},
		Usage:   "Apply all staged changes to AWS",
		Description: `Apply all staged changes (SSM Parameter Store and Secrets Manager) to AWS.

After successful apply, the staged changes are cleared.

Use 'suve stage status' to view all staged changes before applying.
Use 'suve stage param apply' or 'suve stage secret apply' for service-specific changes.

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
				Usage: "Apply even if AWS was modified after staging",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := staging.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	// Check if there are any staged changes
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
		output.Info(cmd.Root().Writer, "No changes staged.")
		return nil
	}

	// Count total staged changes
	totalStaged := len(paramStaged[staging.ServiceParam]) + len(secretStaged[staging.ServiceSecret])

	// Confirm apply
	skipConfirm := cmd.Bool("yes")
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	message := fmt.Sprintf("Apply %d staged change(s) to AWS?", totalStaged)
	confirmed, err := prompter.Confirm(message, skipConfirm)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	r := &Runner{
		Store:           store,
		Stdout:          cmd.Root().Writer,
		Stderr:          cmd.Root().ErrWriter,
		IgnoreConflicts: cmd.Bool("ignore-conflicts"),
	}

	// Initialize strategies only if needed
	if hasParam {
		strat, err := staging.ParamFactory(ctx)
		if err != nil {
			return err
		}
		r.ParamStrategy = strat
	}

	if hasSecret {
		strat, err := staging.SecretFactory(ctx)
		if err != nil {
			return err
		}
		r.SecretStrategy = strat
	}

	return r.Run(ctx)
}

// Run executes the apply command.
func (r *Runner) Run(ctx context.Context) error {
	// Get all staged changes (empty string means all services)
	allStaged, err := r.Store.List("")
	if err != nil {
		return err
	}

	paramStaged := allStaged[staging.ServiceParam]
	secretStaged := allStaged[staging.ServiceSecret]

	// Check for conflicts unless --ignore-conflicts is specified
	if !r.IgnoreConflicts {
		var checks []serviceConflictCheck
		if len(paramStaged) > 0 && r.ParamStrategy != nil {
			checks = append(checks, serviceConflictCheck{
				entries:  paramStaged,
				strategy: r.ParamStrategy,
			})
		}
		if len(secretStaged) > 0 && r.SecretStrategy != nil {
			checks = append(checks, serviceConflictCheck{
				entries:  secretStaged,
				strategy: r.SecretStrategy,
			})
		}

		allConflicts := r.checkAllConflicts(ctx, checks)
		if len(allConflicts) > 0 {
			for _, name := range maputil.SortedKeys(allConflicts) {
				output.Warning(r.Stderr, "conflict detected for %s: AWS was modified after staging", name)
			}
			return fmt.Errorf("apply rejected: %d conflict(s) detected (use --ignore-conflicts to force)", len(allConflicts))
		}
	}

	var totalSucceeded, totalFailed int

	// Apply SSM Parameter Store changes
	if len(paramStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Applying SSM Parameter Store parameters...")
		succeeded, failed := r.applyService(ctx, r.ParamStrategy, paramStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Apply Secrets Manager changes
	if len(secretStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Applying Secrets Manager secrets...")
		succeeded, failed := r.applyService(ctx, r.SecretStrategy, secretStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Summary
	if totalFailed > 0 {
		return fmt.Errorf("applied %d, failed %d", totalSucceeded, totalFailed)
	}

	return nil
}

func (r *Runner) applyService(ctx context.Context, strat staging.ApplyStrategy, staged map[string]staging.Entry) (succeeded, failed int) {
	service := strat.Service()
	serviceName := strat.ServiceName()

	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, name string, entry staging.Entry) (staging.Operation, error) {
		err := strat.Apply(ctx, name, entry)
		return entry.Operation, err
	})

	for _, name := range maputil.SortedKeys(staged) {
		result := results[name]
		if result.Err != nil {
			output.Failed(r.Stderr, serviceName+": "+name, result.Err)
			failed++
		} else {
			switch result.Value {
			case staging.OperationCreate:
				output.Success(r.Stdout, "%s: Created %s", serviceName, name)
			case staging.OperationUpdate:
				output.Success(r.Stdout, "%s: Updated %s", serviceName, name)
			case staging.OperationDelete:
				output.Success(r.Stdout, "%s: Deleted %s", serviceName, name)
			}
			if err := r.Store.Unstage(service, name); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", name, err)
			}
			succeeded++
		}
	}

	return succeeded, failed
}

// checkAllConflicts checks all services for conflicts and returns a combined map of conflicting names.
func (r *Runner) checkAllConflicts(ctx context.Context, checks []serviceConflictCheck) map[string]struct{} {
	allConflicts := make(map[string]struct{})

	for _, check := range checks {
		conflicts := r.checkConflicts(ctx, check.strategy, check.entries)
		for name := range conflicts {
			allConflicts[name] = struct{}{}
		}
	}

	return allConflicts
}

// checkConflicts checks if AWS resources were modified after staging.
// Returns a map of names that have conflicts.
func (r *Runner) checkConflicts(ctx context.Context, strat staging.ApplyStrategy, entries map[string]staging.Entry) map[string]struct{} {
	conflicts := make(map[string]struct{})

	// Separate entries by check type:
	// - Create: check if resource now exists (someone else created it)
	// - Update/Delete with BaseModifiedAt: check if modified after base
	toCheckCreate := make(map[string]staging.Entry)
	toCheckModified := make(map[string]staging.Entry)

	for name, entry := range entries {
		switch {
		case entry.Operation == staging.OperationCreate:
			toCheckCreate[name] = entry
		case (entry.Operation == staging.OperationUpdate || entry.Operation == staging.OperationDelete) && entry.BaseModifiedAt != nil:
			toCheckModified[name] = entry
		}
	}

	if len(toCheckCreate) == 0 && len(toCheckModified) == 0 {
		return conflicts
	}

	// Combine all entries for parallel fetch
	allToCheck := make(map[string]staging.Entry)
	for name, entry := range toCheckCreate {
		allToCheck[name] = entry
	}
	for name, entry := range toCheckModified {
		allToCheck[name] = entry
	}

	// Fetch last modified times in parallel
	results := parallel.ExecuteMap(ctx, allToCheck, func(ctx context.Context, name string, _ staging.Entry) (time.Time, error) {
		return strat.FetchLastModified(ctx, name)
	})

	// Check for conflicts - Create operations
	for name := range toCheckCreate {
		result := results[name]
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on apply anyway)
			continue
		}

		// For Create: if resource now exists (non-zero time), someone else created it
		if !result.Value.IsZero() {
			conflicts[name] = struct{}{}
		}
	}

	// Check for conflicts - Update/Delete operations
	for name, entry := range toCheckModified {
		result := results[name]
		if result.Err != nil {
			// If we can't fetch, assume no conflict (will fail on apply anyway)
			continue
		}

		awsModified := result.Value

		// Zero time means resource doesn't exist - no conflict for delete (already gone)
		if awsModified.IsZero() {
			continue
		}

		// If AWS was modified after the base value was fetched, it's a conflict
		if awsModified.After(*entry.BaseModifiedAt) {
			conflicts[name] = struct{}{}
		}
	}

	return conflicts
}
