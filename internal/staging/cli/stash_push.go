package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
	"github.com/mpyw/suve/internal/staging/store/file"
	usestaging "github.com/mpyw/suve/internal/usecase/staging"
)

// StashPushRunner executes stash push operations using a usecase.
type StashPushRunner struct {
	UseCase   *usestaging.StashPushUseCase
	Stdout    io.Writer
	Stderr    io.Writer
	Encrypted bool // Whether the file is encrypted (for output messages)
}

// StashPushOptions holds options for the stash push command.
type StashPushOptions struct {
	// Service filters the operation to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the agent memory after pushing to file.
	Keep bool
	// Mode determines how to handle existing stash file.
	Mode usestaging.StashPushMode
}

// Run executes the stash push command.
func (r *StashPushRunner) Run(ctx context.Context, opts StashPushOptions) error {
	_, err := r.UseCase.Execute(ctx, usestaging.StashPushInput{
		Service: opts.Service,
		Keep:    opts.Keep,
		Mode:    opts.Mode,
	})
	if err != nil {
		// Check for non-fatal error (state was written but agent cleanup failed)
		var persistErr *usestaging.StashPushError
		if errors.As(err, &persistErr) && persistErr.NonFatal {
			output.Warning(r.Stderr, "%v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if opts.Keep {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes stashed to file (encrypted, kept in memory)")
		} else {
			output.Success(r.Stdout, "Staged changes stashed to file (kept in memory)")
		}
	} else {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes stashed to file (encrypted) and cleared from memory")
		} else {
			output.Success(r.Stdout, "Staged changes stashed to file and cleared from memory")
		}
	}

	// Display warning about plain-text storage only if not encrypted
	if !r.Encrypted {
		output.Warn(r.Stderr, "Secrets are stored as plain text.")
	}

	return nil
}

// stashPushFlags returns the common flags for stash push commands.
func stashPushFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "keep",
			Usage: "Keep staged changes in agent memory after stashing",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Overwrite existing stash without prompt",
		},
		&cli.BoolFlag{
			Name:  "merge",
			Usage: "Merge with existing stash without prompt",
		},
		&cli.BoolFlag{
			Name:  "passphrase-stdin",
			Usage: "Read passphrase from stdin (for scripts/automation)",
		},
	}
}

// stashPushAction creates the action function for stash push commands.
//
//nolint:gocognit,cyclop // Complex but readable as a single flow.
func stashPushAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		agentStore := agent.NewStore(identity.AccountID, identity.Region)

		result, err := lifecycle.ExecuteRead(ctx, agentStore, lifecycle.CmdStashPush, func() (struct{}, error) {
			// V3: Check if stash file(s) already exist
			var (
				anyExists      bool
				totalItemCount int
			)

			if service != "" {
				// Service-specific: check single file
				basicStore, err := file.NewStore(identity.AccountID, identity.Region, service)
				if err != nil {
					return struct{}{}, fmt.Errorf("failed to create file store: %w", err)
				}

				anyExists, err = basicStore.Exists()
				if err != nil {
					return struct{}{}, fmt.Errorf("failed to check stash file: %w", err)
				}

				if anyExists {
					existingState, err := basicStore.Drain(ctx, "", true)
					if err == nil {
						totalItemCount = existingState.TotalCount()
					}
				}
			} else {
				// Global: check all files
				basicStores, err := file.NewStoresForAllServices(identity.AccountID, identity.Region)
				if err != nil {
					return struct{}{}, fmt.Errorf("failed to create file stores: %w", err)
				}

				anyExists, err = file.AnyExists(basicStores)
				if err != nil {
					return struct{}{}, fmt.Errorf("failed to check stash files: %w", err)
				}

				if anyExists {
					existingState, err := file.DrainAll(ctx, basicStores, true)
					if err == nil {
						totalItemCount = existingState.TotalCount()
					}
				}
			}

			// Determine mode based on flags and file existence
			mode := usestaging.StashPushModeMerge // Default to merge (safer)
			forceFlag := cmd.Bool("force")
			mergeFlag := cmd.Bool("merge")

			switch {
			case forceFlag:
				mode = usestaging.StashPushModeOverwrite
			case mergeFlag:
				mode = usestaging.StashPushModeMerge
			default:
				// Only prompt for global push when file exists
				// Service-specific push always merges (preserves other services)
				if anyExists && service == "" && terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
					confirmPrompter := &confirm.Prompter{
						Stdin:  cmd.Root().Reader,
						Stdout: cmd.Root().Writer,
						Stderr: cmd.Root().ErrWriter,
					}

					output.Warning(cmd.Root().ErrWriter, "Stash file(s) already exist with %d item(s).", totalItemCount)

					choice, err := confirmPrompter.ConfirmChoice("How do you want to proceed?", []confirm.Choice{
						{Label: "Merge", Description: "combine with existing stash"},
						{Label: "Overwrite", Description: "replace existing stash"},
						{Label: "Cancel", Description: "abort operation"},
					})
					if err != nil {
						return struct{}{}, fmt.Errorf("failed to get confirmation: %w", err)
					}

					switch choice {
					case 0: // Merge
						mode = usestaging.StashPushModeMerge
					case 1: // Overwrite
						mode = usestaging.StashPushModeOverwrite
					default: // Cancel or error
						output.Info(cmd.Root().Writer, "Operation cancelled.")

						return struct{}{}, nil
					}
				}
			}

			// Get passphrase
			prompter := &passphrase.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
			}

			var pass string

			switch {
			case cmd.Bool("passphrase-stdin"):
				pass, err = prompter.ReadFromStdin()
				if err != nil {
					return struct{}{}, fmt.Errorf("failed to read passphrase from stdin: %w", err)
				}
			case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
				pass, err = prompter.PromptForEncrypt()
				if err != nil {
					if errors.Is(err, passphrase.ErrCancelled) {
						return struct{}{}, nil
					}

					return struct{}{}, fmt.Errorf("failed to get passphrase: %w", err)
				}
			default:
				prompter.WarnNonTTY()
				// pass remains empty = plain text
			}

			// V3: Create service-specific store or composite store
			var fileStore store.FileStore
			if service != "" {
				fileStore, err = file.NewStoreWithPassphrase(identity.AccountID, identity.Region, service, pass)
			} else {
				var stores map[staging.Service]*file.Store

				stores, err = file.NewStoresWithPassphrase(identity.AccountID, identity.Region, pass)
				if err == nil {
					fileStore = file.NewCompositeStore(stores)
				}
			}

			if err != nil {
				return struct{}{}, fmt.Errorf("failed to create file store: %w", err)
			}

			r := &StashPushRunner{
				UseCase: &usestaging.StashPushUseCase{
					AgentStore: agentStore,
					FileStore:  fileStore,
				},
				Stdout:    cmd.Root().Writer,
				Stderr:    cmd.Root().ErrWriter,
				Encrypted: pass != "",
			}

			return struct{}{}, r.Run(ctx, StashPushOptions{
				Service: service,
				Keep:    cmd.Bool("keep"),
				Mode:    mode,
			})
		})
		if err != nil {
			// Handle "nothing to stash" gracefully (agent running but empty)
			if errors.Is(err, usestaging.ErrNothingToStashPush) {
				output.Info(cmd.Root().Writer, "No staged changes to persist.")

				return nil
			}

			return err
		}

		if result.NothingStaged {
			output.Info(cmd.Root().Writer, "No staged changes to persist.")
		}

		return nil
	}
}

// newGlobalStashPushCommand creates a global stash push command that operates on all services.
func newGlobalStashPushCommand() *cli.Command {
	return &cli.Command{
		Name:  "push",
		Usage: "Save staged changes from memory to file",
		Description: `Save staged changes from the in-memory agent to a file.

This command saves the current staging state from the agent daemon
to the persistent file storage (~/.suve/{accountID}/{region}/{param,secret}.json).

By default, the agent's memory is cleared after stashing.
Use --keep to retain the staged changes in memory.

EXAMPLES:
   suve stage stash push                            Save to file and clear agent memory
   suve stage stash push --keep                     Save to file and keep agent memory
   echo "secret" | suve stage stash push --passphrase-stdin   Use passphrase from stdin`,
		Flags:  stashPushFlags(),
		Action: stashPushAction(""), // Empty service = all services
	}
}

// newStashPushCommand creates a service-specific stash push command with the given config.
func newStashPushCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "push",
		Usage: fmt.Sprintf("Save staged %s changes from memory to file", cfg.ItemName),
		Description: fmt.Sprintf(`Save staged %s changes from the in-memory agent to a file.

This command saves the staging state for %ss from the agent daemon
to the persistent file storage (~/.suve/{accountID}/{region}/%s.json).

By default, the %s entries are cleared from agent memory after stashing.
Use --keep to retain them in memory.

EXAMPLES:
   suve stage %s stash push                            Save to file and clear agent memory
   suve stage %s stash push --keep                     Save to file and keep agent memory
   echo "secret" | suve stage %s stash push --passphrase-stdin   Use passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:  stashPushFlags(),
		Action: stashPushAction(service),
	}
}
