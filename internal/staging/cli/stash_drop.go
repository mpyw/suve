package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// StashDropRunner executes stash drop operations for a single service.
type StashDropRunner struct {
	FileStore *file.Store
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run executes the stash drop command for a single service.
func (r *StashDropRunner) Run(_ context.Context) error {
	// Check if file exists
	exists, err := r.FileStore.Exists()
	if err != nil {
		return fmt.Errorf("failed to check stash file: %w", err)
	}

	if !exists {
		return errors.New("no stashed changes to drop")
	}

	// V3: Just delete the file (no need to preserve other services)
	if err := r.FileStore.Delete(); err != nil {
		return fmt.Errorf("failed to delete stash file: %w", err)
	}

	output.Success(r.Stdout, "Stashed %s changes dropped", r.FileStore.Service())

	return nil
}

// GlobalStashDropRunner executes stash drop operations for all services.
type GlobalStashDropRunner struct {
	FileStores map[staging.Service]*file.Store
	Stdout     io.Writer
	Stderr     io.Writer
}

// Run executes the global stash drop command.
func (r *GlobalStashDropRunner) Run(_ context.Context) error {
	// Check if any file exists
	anyExists, err := file.AnyExists(r.FileStores)
	if err != nil {
		return fmt.Errorf("failed to check stash files: %w", err)
	}

	if !anyExists {
		return errors.New("no stashed changes to drop")
	}

	// V3: Delete all files
	if err := file.DeleteAll(r.FileStores); err != nil {
		return fmt.Errorf("failed to delete stash files: %w", err)
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

		_, err = lifecycle.ExecuteFile(ctx, lifecycle.CmdStashDrop, func() (struct{}, error) {
			forceFlag := cmd.Bool("force")

			if service != "" {
				// Service-specific drop
				return struct{}{}, runServiceSpecificDrop(ctx, cmd, identity.AccountID, identity.Region, service, forceFlag)
			}

			// Global drop
			return struct{}{}, runGlobalDrop(ctx, cmd, identity.AccountID, identity.Region, forceFlag)
		})

		return err
	}
}

func runServiceSpecificDrop(ctx context.Context, cmd *cli.Command, accountID, region string, service staging.Service, force bool) error {
	fileStore, err := file.NewStore(accountID, region, service)
	if err != nil {
		return fmt.Errorf("failed to create file store: %w", err)
	}

	// Check if file exists
	exists, err := fileStore.Exists()
	if err != nil {
		return fmt.Errorf("failed to check stash file: %w", err)
	}

	if !exists {
		return fmt.Errorf("no stashed changes for %s", service)
	}

	// Confirm unless --force
	if !force && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
		// Try to count items for confirmation message
		// If encrypted, we can still drop without passphrase
		var target string

		state, err := fileStore.Drain(ctx, "", true)

		switch {
		case errors.Is(err, file.ErrDecryptionFailed):
			// File is encrypted, but we can still drop it
			target = fmt.Sprintf("encrypted %s stash file", service)
		case err != nil:
			return fmt.Errorf("failed to read stash file: %w", err)
		default:
			itemCount := len(state.Entries[service]) + len(state.Tags[service])
			if itemCount == 0 {
				return fmt.Errorf("no stashed changes for %s", service)
			}

			target = fmt.Sprintf("%d stashed %s item(s)", itemCount, service)
		}

		confirmPrompter := &confirm.Prompter{
			Stdin:  cmd.Root().Reader,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		confirmed, err := confirmPrompter.ConfirmDelete(target, false)
		if err != nil {
			return fmt.Errorf("failed to get confirmation: %w", err)
		}

		if !confirmed {
			output.Info(cmd.Root().Writer, "Operation cancelled.")

			return nil
		}
	}

	r := &StashDropRunner{
		FileStore: fileStore,
		Stdout:    cmd.Root().Writer,
		Stderr:    cmd.Root().ErrWriter,
	}

	return r.Run(ctx)
}

func runGlobalDrop(ctx context.Context, cmd *cli.Command, accountID, region string, force bool) error {
	fileStores, err := file.NewStoresForAllServices(accountID, region)
	if err != nil {
		return fmt.Errorf("failed to create file stores: %w", err)
	}

	// Check if any file exists
	anyExists, err := file.AnyExists(fileStores)
	if err != nil {
		return fmt.Errorf("failed to check stash files: %w", err)
	}

	if !anyExists {
		return errors.New("no stashed changes to drop")
	}

	// Confirm unless --force
	if !force && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
		// Try to count items for confirmation message
		var (
			target       string
			totalCount   int
			hasEncrypted bool
		)

		for _, svc := range file.AllServices {
			store := fileStores[svc]

			exists, err := store.Exists()
			if err != nil {
				return fmt.Errorf("failed to check stash file: %w", err)
			}

			if !exists {
				continue
			}

			state, err := store.Drain(ctx, "", true)
			if errors.Is(err, file.ErrDecryptionFailed) {
				hasEncrypted = true

				continue
			} else if err != nil {
				return fmt.Errorf("failed to read stash file: %w", err)
			}

			totalCount += state.TotalCount()
		}

		switch {
		case hasEncrypted && totalCount > 0:
			target = fmt.Sprintf("%d stashed item(s) and encrypted file(s)", totalCount)
		case hasEncrypted:
			target = "encrypted stash file(s)"
		case totalCount > 0:
			target = fmt.Sprintf("%d stashed item(s)", totalCount)
		default:
			return errors.New("no stashed changes to drop")
		}

		confirmPrompter := &confirm.Prompter{
			Stdin:  cmd.Root().Reader,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		confirmed, err := confirmPrompter.ConfirmDelete(target, false)
		if err != nil {
			return fmt.Errorf("failed to get confirmation: %w", err)
		}

		if !confirmed {
			output.Info(cmd.Root().Writer, "Operation cancelled.")

			return nil
		}
	}

	r := &GlobalStashDropRunner{
		FileStores: fileStores,
		Stdout:     cmd.Root().Writer,
		Stderr:     cmd.Root().ErrWriter,
	}

	return r.Run(ctx)
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

This command permanently removes the stashed %s changes.
You will be prompted to confirm unless --force is specified.

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
