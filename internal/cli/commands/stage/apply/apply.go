// Package apply provides the global apply command for applying all staged changes.
package apply

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// Runner executes the apply command.
type Runner struct {
	ParamStrategy  staging.ApplyStrategy
	SecretStrategy staging.ApplyStrategy
	Store          *staging.Store
	Stdout         io.Writer
	Stderr         io.Writer
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

EXAMPLES:
   suve stage apply        Apply all staged changes (with confirmation)
   suve stage apply --yes  Apply without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
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
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
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
