package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// StashDropRunner executes stash drop operations.
type StashDropRunner struct {
	FileStore *file.Store
	Stdout    io.Writer
	Stderr    io.Writer
}

// StashDropOptions holds options for the stash drop command.
type StashDropOptions struct {
	// Service filters the operation to a specific service. Empty means all services.
	Service staging.Service
	// Force skips the confirmation prompt.
	Force bool
}

// Run executes the stash drop command.
func (r *StashDropRunner) Run(ctx context.Context, opts StashDropOptions) error {
	// Check if file exists
	exists, err := r.FileStore.Exists()
	if err != nil {
		return fmt.Errorf("failed to check stash file: %w", err)
	}

	if !exists {
		return errors.New("no stashed changes to drop")
	}

	// For service-specific drop, we need to read, remove service, and write back
	if opts.Service != "" {
		state, err := r.FileStore.Drain(ctx, "", true)
		if err != nil {
			return fmt.Errorf("failed to read stash file: %w", err)
		}

		// Check if the service has any entries
		hasEntries := len(state.Entries[opts.Service]) > 0 || len(state.Tags[opts.Service]) > 0
		if !hasEntries {
			return fmt.Errorf("no stashed changes for %s", opts.Service)
		}

		// Remove the service
		state.RemoveService(opts.Service)

		// If state is now empty, delete the file entirely; otherwise write back remaining state
		updateErr := lo.
			IfF(state.IsEmpty(), func() error { _, err := r.FileStore.Drain(ctx, "", false); return err }).
			ElseF(func() error { return r.FileStore.WriteState(ctx, "", state) })
		if updateErr != nil {
			return fmt.Errorf("failed to update stash file: %w", updateErr)
		}

		output.Success(r.Stdout, "Stashed %s changes dropped", opts.Service)

		return nil
	}

	// Global drop - delete entire file
	_, err = r.FileStore.Drain(ctx, "", false)
	if err != nil {
		return fmt.Errorf("failed to delete stash file: %w", err)
	}

	output.Success(r.Stdout, "All stashed changes dropped")

	return nil
}

// stashDropFlags returns the common flags for stash drop commands.
func stashDropFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Skip confirmation prompt",
		},
	}
}

// stashDropAction creates the action function for stash drop commands.
func stashDropAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		fileStore, err := file.NewStore(identity.AccountID, identity.Region)
		if err != nil {
			return fmt.Errorf("failed to create file store: %w", err)
		}

		// Check if file exists
		exists, err := fileStore.Exists()
		if err != nil {
			return fmt.Errorf("failed to check stash file: %w", err)
		}

		if !exists {
			return errors.New("no stashed changes to drop")
		}

		// Confirm unless --force
		forceFlag := cmd.Bool("force")
		if !forceFlag && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
			// Count items for the message
			state, err := fileStore.Drain(ctx, "", true)
			if err != nil {
				return fmt.Errorf("failed to read stash file: %w", err)
			}

			itemCount := lo.
				If(service != "", len(state.Entries[service])+len(state.Tags[service])).
				Else(state.TotalCount())

			if itemCount == 0 {
				return lo.
					IfF(service != "", func() error { return fmt.Errorf("no stashed changes for %s", service) }).
					ElseF(func() error { return errors.New("no stashed changes to drop") })
			}

			confirmPrompter := &confirm.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			target := lo.
				If(service != "", fmt.Sprintf("%d stashed %s item(s)", itemCount, service)).
				Else(fmt.Sprintf("%d stashed item(s)", itemCount))

			confirmed, err := confirmPrompter.ConfirmDelete(target, false)
			if err != nil {
				return fmt.Errorf("failed to get confirmation: %w", err)
			}

			if !confirmed {
				output.Printf(cmd.Root().Writer, "Operation cancelled.\n")
				return nil
			}
		}

		r := &StashDropRunner{
			FileStore: fileStore,
			Stdout:    cmd.Root().Writer,
			Stderr:    cmd.Root().ErrWriter,
		}

		return r.Run(ctx, StashDropOptions{
			Service: service,
			Force:   forceFlag,
		})
	}
}

// newGlobalStashDropCommand creates a global stash drop command that operates on all services.
func newGlobalStashDropCommand() *cli.Command {
	return &cli.Command{
		Name:  "drop",
		Usage: "Delete stashed changes without restoring",
		Description: `Delete stashed changes from the file without loading them into memory.

This command permanently removes the stashed changes. You will be
prompted to confirm unless --force is specified.

EXAMPLES:
   suve stage stash drop                            Delete all stashed changes
   suve stage stash drop --force                    Delete without confirmation`,
		Flags:  stashDropFlags(),
		Action: stashDropAction(""),
	}
}

// newStashDropCommand creates a service-specific stash drop command with the given config.
func newStashDropCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "drop",
		Usage: fmt.Sprintf("Delete stashed %s changes without restoring", cfg.ItemName),
		Description: fmt.Sprintf(`Delete stashed %s changes from the file without loading them into memory.

This command permanently removes the stashed %s changes. Other services'
stashed changes are preserved. You will be prompted to confirm unless
--force is specified.

EXAMPLES:
   suve stage %s stash drop                            Delete stashed %s changes
   suve stage %s stash drop --force                    Delete without confirmation`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName),
		Flags:  stashDropFlags(),
		Action: stashDropAction(service),
	}
}
