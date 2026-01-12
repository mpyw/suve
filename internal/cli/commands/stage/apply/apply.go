// Package apply provides the global apply command for applying all staged changes.
package apply

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent"
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
	Store           staging.StoreReadWriteOperator
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
	identity, err := infra.GetAWSIdentity(ctx)
	if err != nil {
		return fmt.Errorf("failed to get AWS identity: %w", err)
	}
	store := agent.NewStore(identity.AccountID, identity.Region)

	// Check if there are any staged changes
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
		output.Info(cmd.Root().Writer, "No changes staged.")
		return nil
	}

	// Count total staged changes
	totalStaged := len(paramStaged[staging.ServiceParam]) + len(secretStaged[staging.ServiceSecret]) +
		len(paramTagStaged[staging.ServiceParam]) + len(secretTagStaged[staging.ServiceSecret])

	// Confirm apply
	skipConfirm := cmd.Bool("yes")
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	prompter.AccountID = identity.AccountID
	prompter.Region = identity.Region
	prompter.Profile = identity.Profile

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
	allStaged, err := r.Store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	allTagStaged, err := r.Store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	paramStaged := allStaged[staging.ServiceParam]
	secretStaged := allStaged[staging.ServiceSecret]
	paramTagStaged := allTagStaged[staging.ServiceParam]
	secretTagStaged := allTagStaged[staging.ServiceSecret]

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

	// Apply SSM Parameter Store tag changes
	if len(paramTagStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Applying SSM Parameter Store tags...")
		succeeded, failed := r.applyTagService(ctx, r.ParamStrategy, paramTagStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Apply Secrets Manager tag changes
	if len(secretTagStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Applying Secrets Manager tags...")
		succeeded, failed := r.applyTagService(ctx, r.SecretStrategy, secretTagStaged)
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
			if err := r.Store.UnstageEntry(ctx, service, name); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", name, err)
			}
			succeeded++
		}
	}

	return succeeded, failed
}

func (r *Runner) applyTagService(ctx context.Context, strat staging.ApplyStrategy, staged map[string]staging.TagEntry) (succeeded, failed int) {
	service := strat.Service()
	serviceName := strat.ServiceName()

	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, name string, tagEntry staging.TagEntry) (struct{}, error) {
		err := strat.ApplyTags(ctx, name, tagEntry)
		return struct{}{}, err
	})

	for _, name := range maputil.SortedKeys(staged) {
		tagEntry := staged[name]
		result := results[name]
		if result.Err != nil {
			output.Failed(r.Stderr, serviceName+": "+name+" (tags)", result.Err)
			failed++
		} else {
			output.Success(r.Stdout, "%s: Tagged %s%s", serviceName, name, formatTagApplySummary(tagEntry))
			if err := r.Store.UnstageTag(ctx, service, name); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s tags: %v", name, err)
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
func (r *Runner) checkAllConflicts(ctx context.Context, checks []serviceConflictCheck) map[string]struct{} {
	allConflicts := make(map[string]struct{})

	for _, check := range checks {
		conflicts := staging.CheckConflicts(ctx, check.strategy, check.entries)
		for name := range conflicts {
			allConflicts[name] = struct{}{}
		}
	}

	return allConflicts
}
