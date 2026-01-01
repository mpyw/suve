// Package push provides the global push command for applying all staged changes.
package push

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
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
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(cmd.Root().Writer, yellow("No changes staged."))
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

	if len(ssmStaged) == 0 && len(smStaged) == 0 {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No changes staged."))
		return nil
	}

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
	// Sort names for consistent output
	var names []string
	for name := range staged {
		names = append(names, name)
	}
	sort.Strings(names)

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	for _, name := range names {
		entry := staged[name]

		var pushErr error
		switch entry.Operation {
		case stage.OperationSet:
			pushErr = r.pushSSMSet(ctx, name, entry.Value)
		case stage.OperationDelete:
			pushErr = r.pushSSMDelete(ctx, name)
		default:
			pushErr = fmt.Errorf("unknown operation: %s", entry.Operation)
		}

		if pushErr != nil {
			_, _ = fmt.Fprintf(r.Stderr, "%s SSM: %s: %v\n", red("Failed"), name, pushErr)
			failed++
		} else {
			if entry.Operation == stage.OperationSet {
				_, _ = fmt.Fprintf(r.Stdout, "%s SSM: Set %s\n", green("✓"), name)
			} else {
				_, _ = fmt.Fprintf(r.Stdout, "%s SSM: Deleted %s\n", green("✓"), name)
			}
			// Clear staging for this item
			if err := r.Store.Unstage(stage.ServiceSSM, name); err != nil {
				_, _ = fmt.Fprintf(r.Stderr, "Warning: failed to clear staging for %s: %v\n", name, err)
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
	if err == nil && existing.Parameter != nil {
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
	// Sort names for consistent output
	var names []string
	for name := range staged {
		names = append(names, name)
	}
	sort.Strings(names)

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	for _, name := range names {
		entry := staged[name]

		var pushErr error
		switch entry.Operation {
		case stage.OperationSet:
			pushErr = r.pushSMSet(ctx, name, entry.Value)
		case stage.OperationDelete:
			pushErr = r.pushSMDelete(ctx, name, entry)
		default:
			pushErr = fmt.Errorf("unknown operation: %s", entry.Operation)
		}

		if pushErr != nil {
			_, _ = fmt.Fprintf(r.Stderr, "%s SM: %s: %v\n", red("Failed"), name, pushErr)
			failed++
		} else {
			if entry.Operation == stage.OperationSet {
				_, _ = fmt.Fprintf(r.Stdout, "%s SM: Set %s\n", green("✓"), name)
			} else {
				_, _ = fmt.Fprintf(r.Stdout, "%s SM: Deleted %s\n", green("✓"), name)
			}
			// Clear staging for this item
			if err := r.Store.Unstage(stage.ServiceSM, name); err != nil {
				_, _ = fmt.Fprintf(r.Stderr, "Warning: failed to clear staging for %s: %v\n", name, err)
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
