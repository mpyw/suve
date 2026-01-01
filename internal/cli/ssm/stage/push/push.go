// Package push provides the SSM push command for applying staged changes.
package push

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/stage"
)

// Client is the interface for the push command.
type Client interface {
	ssmapi.PutParameterAPI
	ssmapi.DeleteParameterAPI
	ssmapi.GetParameterAPI
}

// Runner executes the push command.
type Runner struct {
	Client Client
	Store  *stage.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the push command.
type Options struct {
	Name string // Optional: push only this parameter, otherwise push all
}

// Command returns the push command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     "Apply staged parameter changes to AWS",
		ArgsUsage: "[name]",
		Description: `Apply all staged SSM parameter changes to AWS.

If a parameter name is specified, only that parameter's staged changes are applied.
Otherwise, all staged SSM parameter changes are applied.

After successful push, the staged changes are cleared.

Use 'suve ssm status' to view staged changes before pushing.

EXAMPLES:
   suve ssm push                    Push all staged SSM changes
   suve ssm push /app/config/db     Push only the specified parameter`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	opts := Options{}
	if cmd.Args().Len() > 0 {
		opts.Name = cmd.Args().First()
	}

	return r.Run(ctx, opts)
}

// Run executes the push command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	// Get staged changes
	stagedMap, err := r.Store.List(stage.ServiceSSM)
	if err != nil {
		return err
	}

	staged := stagedMap[stage.ServiceSSM]
	if len(staged) == 0 {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No SSM changes staged."))
		return nil
	}

	// If specific name requested, filter to just that
	if opts.Name != "" {
		entry, exists := staged[opts.Name]
		if !exists {
			return fmt.Errorf("parameter %s is not staged", opts.Name)
		}
		staged = map[string]stage.Entry{opts.Name: entry}
	}

	// Sort names for consistent output
	var names []string
	for name := range staged {
		names = append(names, name)
	}
	sort.Strings(names)

	// Apply each change
	var succeeded, failed int
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	for _, name := range names {
		entry := staged[name]

		var pushErr error
		switch entry.Operation {
		case stage.OperationSet:
			pushErr = r.pushSet(ctx, name, entry.Value)
		case stage.OperationDelete:
			pushErr = r.pushDelete(ctx, name)
		default:
			pushErr = fmt.Errorf("unknown operation: %s", entry.Operation)
		}

		if pushErr != nil {
			_, _ = fmt.Fprintf(r.Stderr, "%s %s: %v\n", red("Failed"), name, pushErr)
			failed++
		} else {
			if entry.Operation == stage.OperationSet {
				_, _ = fmt.Fprintf(r.Stdout, "%s Set %s\n", green("✓"), name)
			} else {
				_, _ = fmt.Fprintf(r.Stdout, "%s Deleted %s\n", green("✓"), name)
			}
			// Clear staging for this item
			if err := r.Store.Unstage(stage.ServiceSSM, name); err != nil {
				_, _ = fmt.Fprintf(r.Stderr, "Warning: failed to clear staging for %s: %v\n", name, err)
			}
			succeeded++
		}
	}

	// Summary
	if failed > 0 {
		return fmt.Errorf("pushed %d, failed %d", succeeded, failed)
	}

	return nil
}

func (r *Runner) pushSet(ctx context.Context, name, value string) error {
	// Try to get existing parameter to preserve type
	paramType := types.ParameterTypeString
	existing, err := r.Client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: lo.ToPtr(name),
	})
	if err == nil && existing.Parameter != nil {
		paramType = existing.Parameter.Type
	}

	_, err = r.Client.PutParameter(ctx, &ssm.PutParameterInput{
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

func (r *Runner) pushDelete(ctx context.Context, name string) error {
	_, err := r.Client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: lo.ToPtr(name),
	})
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}
	return nil
}
