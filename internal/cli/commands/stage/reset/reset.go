// Package reset provides the global reset command for unstaging all changes.
package reset

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
)

// Runner executes the reset command.
type Runner struct {
	Store  store.ReadWriteOperator
	Stdout io.Writer
	Stderr io.Writer
}

// Command returns the global reset command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "reset",
		Usage: "Unstage all changes",
		Description: `Remove all staged changes (SSM Parameter Store and Secrets Manager) from the staging area.

This does not affect AWS - it only clears the local staging area.

Use 'suve stage param reset' or 'suve stage secret reset' for service-specific operations.

EXAMPLES:
   suve stage reset --all    Unstage all changes (SSM Parameter Store and Secrets Manager)`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all changes (required)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	// Require --all flag for safety
	if !cmd.Bool("all") {
		output.Warning(cmd.Root().ErrWriter, "no effect without --all flag")
		output.Hint(cmd.Root().ErrWriter, "Use 'suve stage reset --all' to unstage all changes")

		return nil
	}

	identity, err := infra.GetAWSIdentity(ctx)
	if err != nil {
		return fmt.Errorf("failed to get AWS identity: %w", err)
	}

	store := agent.NewStore(identity.AccountID, identity.Region)

	result, err := lifecycle.ExecuteRead0(ctx, store, lifecycle.CmdReset, func() error {
		r := &Runner{
			Store:  store,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		return r.Run(ctx)
	})
	if err != nil {
		return err
	}

	if result.NothingStaged {
		output.Info(cmd.Root().Writer, "No changes staged.")
	}

	return nil
}

// Run executes the reset command.
func (r *Runner) Run(ctx context.Context) error {
	// Get counts before reset
	staged, err := r.Store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	tagStaged, err := r.Store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	paramEntryCount := len(staged[staging.ServiceParam])
	paramTagCount := len(tagStaged[staging.ServiceParam])
	paramCount := paramEntryCount + paramTagCount

	secretEntryCount := len(staged[staging.ServiceSecret])
	secretTagCount := len(tagStaged[staging.ServiceSecret])
	secretCount := secretEntryCount + secretTagCount

	totalCount := paramCount + secretCount

	// Always call UnstageAll to trigger daemon auto-shutdown check
	// Empty service ("") clears both SSM Parameter Store and Secrets Manager
	// Use hint for context-aware shutdown message
	if hinted, ok := r.Store.(store.HintedUnstager); ok {
		if err := hinted.UnstageAllWithHint(ctx, "", store.HintReset); err != nil {
			return err
		}
	} else if err := r.Store.UnstageAll(ctx, ""); err != nil {
		return err
	}

	if totalCount == 0 {
		output.Info(r.Stdout, "No changes staged.")

		return nil
	}

	output.Success(r.Stdout, "Unstaged all changes (%d SSM Parameter Store, %d Secrets Manager)", paramCount, secretCount)

	return nil
}
