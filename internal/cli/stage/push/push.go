// Package push provides the global push command for applying all staged changes.
package push

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/stage"
)

// SSMClient is the interface for SSM operations.
type SSMClient interface {
	ssmapi.PutParameterAPI
	ssmapi.DeleteParameterAPI
	ssmapi.GetParameterAPI
}

// SMClient is the interface for SM operations.
type SMClient interface {
	smapi.PutSecretValueAPI
	smapi.DeleteSecretAPI
}

// Runner executes the push command.
type Runner struct {
	SSMClient SSMClient
	SMClient  SMClient
	Store     *stage.Store
	Stdout    io.Writer
	Stderr    io.Writer
}

// Command returns the global push command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "push",
		Usage: "Apply all staged changes to AWS",
		Description: `Apply all staged changes (both SSM and SM) to AWS.

After successful push, the staged changes are cleared.

Use 'suve stage status' to view all staged changes before pushing.
Use 'suve ssm stage push' or 'suve sm stage push' to push service-specific changes.

EXAMPLES:
   suve stage push    Push all staged changes (SSM and SM)`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	// Check if there are any staged changes
	ssmStaged, err := store.List(stage.ServiceSSM)
	if err != nil {
		return err
	}
	smStaged, err := store.List(stage.ServiceSM)
	if err != nil {
		return err
	}

	hasSSM := len(ssmStaged[stage.ServiceSSM]) > 0
	hasSM := len(smStaged[stage.ServiceSM]) > 0

	if !hasSSM && !hasSM {
		output.Info(cmd.Root().Writer, "No changes staged.")
		return nil
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	// Initialize clients only if needed
	if hasSSM {
		ssmClient, err := awsutil.NewSSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize SSM client: %w", err)
		}
		r.SSMClient = ssmClient
	}

	if hasSM {
		smClient, err := awsutil.NewSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize SM client: %w", err)
		}
		r.SMClient = smClient
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

	ssmStaged := allStaged[stage.ServiceSSM]
	smStaged := allStaged[stage.ServiceSM]

	var totalSucceeded, totalFailed int

	// Push SSM changes
	if len(ssmStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Pushing SSM parameters...")
		succeeded, failed := r.pushSSM(ctx, ssmStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Push SM changes
	if len(smStaged) > 0 {
		_, _ = fmt.Fprintln(r.Stdout, "Pushing SM secrets...")
		succeeded, failed := r.pushSM(ctx, smStaged)
		totalSucceeded += succeeded
		totalFailed += failed
	}

	// Summary
	if totalFailed > 0 {
		return fmt.Errorf("pushed %d, failed %d", totalSucceeded, totalFailed)
	}

	return nil
}

func (r *Runner) pushSSM(ctx context.Context, staged map[string]stage.Entry) (succeeded, failed int) {
	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, name string, entry stage.Entry) (stage.Operation, error) {
		var err error
		switch entry.Operation {
		case stage.OperationSet:
			err = r.pushSSMSet(ctx, name, entry.Value)
		case stage.OperationDelete:
			err = r.pushSSMDelete(ctx, name)
		default:
			err = fmt.Errorf("unknown operation: %s", entry.Operation)
		}
		return entry.Operation, err
	})

	for _, name := range maputil.SortedKeys(staged) {
		result := results[name]
		if result.Err != nil {
			output.Failed(r.Stderr, "SSM: "+name, result.Err)
			failed++
		} else {
			if result.Value == stage.OperationSet {
				output.Success(r.Stdout, "SSM: Set %s", name)
			} else {
				output.Success(r.Stdout, "SSM: Deleted %s", name)
			}
			if err := r.Store.Unstage(stage.ServiceSSM, name); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", name, err)
			}
			succeeded++
		}
	}

	return succeeded, failed
}

func (r *Runner) pushSSMSet(ctx context.Context, name, value string) error {
	// Try to get existing parameter to preserve type
	paramType := types.ParameterTypeString
	existing, err := r.SSMClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		// Only proceed with String type if parameter doesn't exist
		var pnf *types.ParameterNotFound
		if !errors.As(err, &pnf) {
			return fmt.Errorf("failed to get existing parameter: %w", err)
		}
	} else if existing.Parameter != nil {
		paramType = existing.Parameter.Type
	}

	_, err = r.SSMClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      lo.ToPtr(name),
		Value:     lo.ToPtr(value),
		Type:      paramType,
		Overwrite: lo.ToPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}
	return nil
}

func (r *Runner) pushSSMDelete(ctx context.Context, name string) error {
	_, err := r.SSMClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}
	return nil
}

func (r *Runner) pushSM(ctx context.Context, staged map[string]stage.Entry) (succeeded, failed int) {
	results := parallel.ExecuteMap(ctx, staged, func(ctx context.Context, name string, entry stage.Entry) (stage.Operation, error) {
		var err error
		switch entry.Operation {
		case stage.OperationSet:
			err = r.pushSMSet(ctx, name, entry.Value)
		case stage.OperationDelete:
			err = r.pushSMDelete(ctx, name, entry)
		default:
			err = fmt.Errorf("unknown operation: %s", entry.Operation)
		}
		return entry.Operation, err
	})

	for _, name := range maputil.SortedKeys(staged) {
		result := results[name]
		if result.Err != nil {
			output.Failed(r.Stderr, "SM: "+name, result.Err)
			failed++
		} else {
			if result.Value == stage.OperationSet {
				output.Success(r.Stdout, "SM: Set %s", name)
			} else {
				output.Success(r.Stdout, "SM: Deleted %s", name)
			}
			if err := r.Store.Unstage(stage.ServiceSM, name); err != nil {
				output.Warning(r.Stderr, "failed to clear staging for %s: %v", name, err)
			}
			succeeded++
		}
	}

	return succeeded, failed
}

func (r *Runner) pushSMSet(ctx context.Context, name, value string) error {
	_, err := r.SMClient.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     lo.ToPtr(name),
		SecretString: lo.ToPtr(value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	return nil
}

func (r *Runner) pushSMDelete(ctx context.Context, name string, entry stage.Entry) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: lo.ToPtr(name),
	}

	// Apply delete options if present
	if entry.DeleteOptions != nil {
		if entry.DeleteOptions.Force {
			input.ForceDeleteWithoutRecovery = lo.ToPtr(true)
		} else if entry.DeleteOptions.RecoveryWindow > 0 {
			input.RecoveryWindowInDays = lo.ToPtr(int64(entry.DeleteOptions.RecoveryWindow))
		}
	}

	_, err := r.SMClient.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}
