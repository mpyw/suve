// Package push provides the global push command for applying all staged changes.
package push

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// Runner executes the push command.
type Runner struct {
	ParamStrategy  staging.PushStrategy
	SecretStrategy staging.PushStrategy
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
   suve stage apply    Apply all staged changes (SSM Parameter Store and Secrets Manager)`,
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

// Run executes the push command.
func (r *Runner) Run(ctx context.Context) error {
	// Get all staged changes (empty string means all services)
	allStaged, err := r.Store.List("")
	if err != nil {
		return err
	}

	paramStaged := allStaged[staging.ServiceParam]
	secretStaged := allStaged[staging.ServiceSecret]

	var totalSucceeded, totalFailed int

	// Push SSM Parameter Store changes
	if len(paramStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Pushing SSM Parameter Store parameters...")
		succeeded, failed := r.pushService(ctx, r.ParamStrategy, paramStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Push Secrets Manager changes
	if len(secretStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Pushing Secrets Manager secrets...")
		succeeded, failed := r.pushService(ctx, r.SecretStrategy, secretStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Summary
	if totalFailed > 0 {
		return fmt.Errorf("pushed %d, failed %d", totalSucceeded, totalFailed)
	}

	return nil
}

func (r *Runner) pushService(ctx context.Context, strat staging.PushStrategy, staged map[string]staging.Entry) (succeeded, failed int) {
	service := strat.Service()
	serviceName := strat.ServiceName()

	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, name string, entry staging.Entry) (staging.Operation, error) {
		err := strat.Push(ctx, name, entry)
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
