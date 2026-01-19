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
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// GlobalDropRunner executes global stash drop (delete entire file).
type GlobalDropRunner struct {
	FileStore *file.Store
	Stdout    io.Writer
}

// Run deletes the stash file without reading its contents.
func (r *GlobalDropRunner) Run() error {
	if err := r.FileStore.Delete(); err != nil {
		return fmt.Errorf("failed to delete stash file: %w", err)
	}

	output.Success(r.Stdout, "All stashed changes dropped")

	return nil
}

// ServiceDropRunner executes service-specific stash drop.
type ServiceDropRunner struct {
	FileStore *file.Store
	Service   staging.Service
	Stdout    io.Writer
}

// Run removes the specified service from the stash file.
func (r *ServiceDropRunner) Run(ctx context.Context) error {
	state, err := r.FileStore.Drain(ctx, "", true)
	if err != nil {
		return fmt.Errorf("failed to read stash file: %w", err)
	}

	// Check if the service has any entries
	hasEntries := len(state.Entries[r.Service]) > 0 || len(state.Tags[r.Service]) > 0
	if !hasEntries {
		return fmt.Errorf("no stashed changes for %s", r.Service)
	}

	// Remove the service
	state.RemoveService(r.Service)

	// If state is now empty, delete the file entirely; otherwise write back remaining state
	updateErr := lo.
		IfF(state.IsEmpty(), func() error {
			return r.FileStore.Delete()
		}).
		ElseF(func() error { return r.FileStore.WriteState(ctx, "", state) })
	if updateErr != nil {
		return fmt.Errorf("failed to update stash file: %w", updateErr)
	}

	output.Success(r.Stdout, "Stashed %s changes dropped", r.Service)

	return nil
}

// dropConfirmer handles confirmation prompts for stash drop operations.
type dropConfirmer struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// confirmGlobalDrop prompts for confirmation of global drop.
// If isEncrypted is true, shows a message without item count.
// If itemCount is provided (>= 0), shows the count in the message.
func (c *dropConfirmer) confirmGlobalDrop(isEncrypted bool, itemCount int) (bool, error) {
	prompter := &confirm.Prompter{
		Stdin:  c.stdin,
		Stdout: c.stdout,
		Stderr: c.stderr,
	}

	var target string
	if isEncrypted {
		target = "encrypted stash file"
	} else {
		target = fmt.Sprintf("%d stashed item(s)", itemCount)
	}

	return prompter.ConfirmDelete(target, false)
}

// confirmServiceDrop prompts for confirmation of service-specific drop.
func (c *dropConfirmer) confirmServiceDrop(service staging.Service, itemCount int) (bool, error) {
	prompter := &confirm.Prompter{
		Stdin:  c.stdin,
		Stdout: c.stdout,
		Stderr: c.stderr,
	}

	target := fmt.Sprintf("%d stashed %s item(s)", itemCount, service)

	return prompter.ConfirmDelete(target, false)
}

// globalStashDropAction creates the action function for global stash drop.
func globalStashDropAction() func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		scope := staging.AWSScope(identity.AccountID, identity.Region)

		return lifecycle.ExecuteFile0(ctx, lifecycle.CmdStashDrop, func() error {
			fileStore, err := file.NewStore(scope)
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

			// Confirm unless --yes
			skipConfirm := cmd.Bool("yes")
			if !skipConfirm && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
				// Check if encrypted to decide confirmation message
				isEncrypted, err := fileStore.IsEncrypted()
				if err != nil {
					return fmt.Errorf("failed to check file encryption: %w", err)
				}

				confirmer := &dropConfirmer{
					stdin:  cmd.Root().Reader,
					stdout: cmd.Root().Writer,
					stderr: cmd.Root().ErrWriter,
				}

				var confirmed bool
				if isEncrypted {
					// Can't count items without decryption, show generic message
					confirmed, err = confirmer.confirmGlobalDrop(true, 0)
				} else {
					// Read to count items for confirmation message
					state, readErr := fileStore.Drain(ctx, "", true)
					if readErr != nil {
						return fmt.Errorf("failed to read stash file: %w", readErr)
					}

					itemCount := state.TotalCount()
					if itemCount == 0 {
						return errors.New("no stashed changes to drop")
					}

					confirmed, err = confirmer.confirmGlobalDrop(false, itemCount)
				}

				if err != nil {
					return fmt.Errorf("failed to get confirmation: %w", err)
				}

				if !confirmed {
					output.Info(cmd.Root().Writer, "Operation cancelled.")

					return nil
				}
			}

			r := &GlobalDropRunner{
				FileStore: fileStore,
				Stdout:    cmd.Root().Writer,
			}

			return r.Run()
		})
	}
}

// serviceStashDropAction creates the action function for service-specific stash drop.
func serviceStashDropAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		scope := staging.AWSScope(identity.AccountID, identity.Region)

		return lifecycle.ExecuteFile0(ctx, lifecycle.CmdStashDrop, func() error {
			// Use fileStoreForReading which handles passphrase prompting for encrypted files
			fileStore, err := fileStoreForReading(cmd, scope, true)
			if err != nil {
				return err
			}

			// Confirm unless --yes
			skipConfirm := cmd.Bool("yes")
			if !skipConfirm && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
				// Count items for the message
				state, err := fileStore.Drain(ctx, "", true)
				if err != nil {
					return fmt.Errorf("failed to read stash file: %w", err)
				}

				itemCount := len(state.Entries[service]) + len(state.Tags[service])
				if itemCount == 0 {
					return fmt.Errorf("no stashed changes for %s", service)
				}

				confirmer := &dropConfirmer{
					stdin:  cmd.Root().Reader,
					stdout: cmd.Root().Writer,
					stderr: cmd.Root().ErrWriter,
				}

				confirmed, err := confirmer.confirmServiceDrop(service, itemCount)
				if err != nil {
					return fmt.Errorf("failed to get confirmation: %w", err)
				}

				if !confirmed {
					output.Info(cmd.Root().Writer, "Operation cancelled.")

					return nil
				}
			}

			r := &ServiceDropRunner{
				FileStore: fileStore,
				Service:   service,
				Stdout:    cmd.Root().Writer,
			}

			return r.Run(ctx)
		})
	}
}

// stashDropFlags returns the common flags for stash drop commands.
func stashDropFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt",
		},
	}
}

// serviceStashDropFlags returns flags for service-specific stash drop commands.
func serviceStashDropFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt",
		},
		&cli.BoolFlag{
			Name:  "passphrase-stdin",
			Usage: "Read passphrase from stdin (for scripts/automation)",
		},
	}
}

// newGlobalStashDropCommand creates a global stash drop command that operates on all services.
func newGlobalStashDropCommand() *cli.Command {
	return &cli.Command{
		Name:  "drop",
		Usage: "Delete stashed changes without restoring",
		Description: `Delete stashed changes from the file without loading them into memory.

This command permanently removes the stashed changes. You will be
prompted to confirm unless --yes is specified.

For encrypted stash files, no passphrase is required since the file
is simply deleted without reading its contents.

EXAMPLES:
   suve stage stash drop                            Delete all stashed changes
   suve stage stash drop --yes                      Delete without confirmation`,
		Flags:  stashDropFlags(),
		Action: globalStashDropAction(),
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
--yes is specified.

For encrypted stash files, you will be prompted for the passphrase to
decrypt and re-encrypt the file with remaining changes.

EXAMPLES:
   suve stage %s stash drop                            Delete stashed %s changes
   suve stage %s stash drop --yes                      Delete without confirmation`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName),
		Flags:  serviceStashDropFlags(),
		Action: serviceStashDropAction(service),
	}
}
