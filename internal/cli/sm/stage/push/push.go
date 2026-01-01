// Package push provides the SM push command for applying staged changes.
package push

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/stage"
)

// Client is the interface for the push command.
type Client interface {
	smapi.PutSecretValueAPI
	smapi.DeleteSecretAPI
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
	Name string // Optional: push only this secret, otherwise push all
}

// Command returns the push command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     "Apply staged secret changes to AWS",
		ArgsUsage: "[name]",
		Description: `Apply all staged Secrets Manager changes to AWS.

If a secret name is specified, only that secret's staged changes are applied.
Otherwise, all staged SM secret changes are applied.

After successful push, the staged changes are cleared.

Use 'suve sm status' to view staged changes before pushing.

EXAMPLES:
   suve sm push              Push all staged SM changes
   suve sm push my-secret    Push only the specified secret`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSMClient(ctx)
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
	stagedMap, err := r.Store.List(stage.ServiceSM)
	if err != nil {
		return err
	}

	staged := stagedMap[stage.ServiceSM]
	if len(staged) == 0 {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No SM changes staged."))
		return nil
	}

	// If specific name requested, filter to just that
	if opts.Name != "" {
		entry, exists := staged[opts.Name]
		if !exists {
			return fmt.Errorf("secret %s is not staged", opts.Name)
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
			pushErr = r.pushDelete(ctx, name, entry)
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
			if err := r.Store.Unstage(stage.ServiceSM, name); err != nil {
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
	_, err := r.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     lo.ToPtr(name),
		SecretString: lo.ToPtr(value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	return nil
}

func (r *Runner) pushDelete(ctx context.Context, name string, entry stage.Entry) error {
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

	_, err := r.Client.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}
