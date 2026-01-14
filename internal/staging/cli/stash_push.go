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

// stashPushExistenceResult holds the result of checking stash file existence.
type stashPushExistenceResult struct {
	exists    bool
	itemCount int
}

// checkStashExistence checks if stash file(s) already exist and counts items.
func checkStashExistence(ctx context.Context, identity *infra.AWSIdentity, service staging.Service) (*stashPushExistenceResult, error) {
	if service != "" {
		return checkServiceStashExistence(ctx, identity, service)
	}

	return checkGlobalStashExistence(ctx, identity)
}

func checkServiceStashExistence(ctx context.Context, identity *infra.AWSIdentity, service staging.Service) (*stashPushExistenceResult, error) {
	basicStore, err := file.NewStore(identity.AccountID, identity.Region, service)
	if err != nil {
		return nil, fmt.Errorf("failed to create file store: %w", err)
	}

	exists, err := basicStore.Exists()
	if err != nil {
		return nil, fmt.Errorf("failed to check stash file: %w", err)
	}

	result := &stashPushExistenceResult{exists: exists}

	if exists {
		existingState, err := basicStore.Drain(ctx, "", true)
		if err == nil {
			result.itemCount = existingState.TotalCount()
		}
	}

	return result, nil
}

func checkGlobalStashExistence(ctx context.Context, identity *infra.AWSIdentity) (*stashPushExistenceResult, error) {
	basicStores, err := file.NewStoresForAllServices(identity.AccountID, identity.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to create file stores: %w", err)
	}

	exists, err := file.AnyExists(basicStores)
	if err != nil {
		return nil, fmt.Errorf("failed to check stash files: %w", err)
	}

	result := &stashPushExistenceResult{exists: exists}

	if exists {
		existingState, err := file.DrainAll(ctx, basicStores, true)
		if err == nil {
			result.itemCount = existingState.TotalCount()
		}
	}

	return result, nil
}

// stashPushModeResult holds the result of determining push mode.
type stashPushModeResult struct {
	mode      usestaging.StashPushMode
	cancelled bool
}

// determineStashPushMode determines the push mode based on flags and user prompt.
func determineStashPushMode(cmd *cli.Command, service staging.Service, existence *stashPushExistenceResult) (*stashPushModeResult, error) {
	forceFlag := cmd.Bool("force")
	mergeFlag := cmd.Bool("merge")

	switch {
	case forceFlag:
		return &stashPushModeResult{mode: usestaging.StashPushModeOverwrite}, nil
	case mergeFlag:
		return &stashPushModeResult{mode: usestaging.StashPushModeMerge}, nil
	default:
		return promptStashPushMode(cmd, service, existence)
	}
}

func promptStashPushMode(cmd *cli.Command, service staging.Service, existence *stashPushExistenceResult) (*stashPushModeResult, error) {
	// Only prompt for global push when file exists
	// Service-specific push always merges (preserves other services)
	if !existence.exists || service != "" || !terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
		return &stashPushModeResult{mode: usestaging.StashPushModeMerge}, nil
	}

	confirmPrompter := &confirm.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	output.Warning(cmd.Root().ErrWriter, "Stash file(s) already exist with %d item(s).", existence.itemCount)

	choice, err := confirmPrompter.ConfirmChoice("How do you want to proceed?", []confirm.Choice{
		{Label: "Merge", Description: "combine with existing stash"},
		{Label: "Overwrite", Description: "replace existing stash"},
		{Label: "Cancel", Description: "abort operation"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get confirmation: %w", err)
	}

	switch choice {
	case 0: // Merge
		return &stashPushModeResult{mode: usestaging.StashPushModeMerge}, nil
	case 1: // Overwrite
		return &stashPushModeResult{mode: usestaging.StashPushModeOverwrite}, nil
	default: // Cancel
		output.Info(cmd.Root().Writer, "Operation cancelled.")

		return &stashPushModeResult{cancelled: true}, nil
	}
}

// stashPushPassphraseResult holds the result of getting passphrase.
type stashPushPassphraseResult struct {
	passphrase string
	cancelled  bool
}

// getStashPushPassphrase gets the passphrase from stdin or prompt.
func getStashPushPassphrase(cmd *cli.Command) (*stashPushPassphraseResult, error) {
	prompter := &passphrase.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	switch {
	case cmd.Bool("passphrase-stdin"):
		pass, err := prompter.ReadFromStdin()
		if err != nil {
			return nil, fmt.Errorf("failed to read passphrase from stdin: %w", err)
		}

		return &stashPushPassphraseResult{passphrase: pass}, nil

	case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
		pass, err := prompter.PromptForEncrypt()
		if err != nil {
			if errors.Is(err, passphrase.ErrCancelled) {
				return &stashPushPassphraseResult{cancelled: true}, nil
			}

			return nil, fmt.Errorf("failed to get passphrase: %w", err)
		}

		return &stashPushPassphraseResult{passphrase: pass}, nil

	default:
		prompter.WarnNonTTY()

		return &stashPushPassphraseResult{}, nil // Empty passphrase = plain text
	}
}

// createStashPushFileStore creates a file store for stash push operation.
func createStashPushFileStore(identity *infra.AWSIdentity, service staging.Service, pass string) (store.FileStore, error) {
	if service != "" {
		return file.NewStoreWithPassphrase(identity.AccountID, identity.Region, service, pass)
	}

	stores, err := file.NewStoresWithPassphrase(identity.AccountID, identity.Region, pass)
	if err != nil {
		return nil, err
	}

	return file.NewCompositeStore(stores), nil
}

// stashPushAction creates the action function for stash push commands.
func stashPushAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		agentStore := agent.NewStore(identity.AccountID, identity.Region)

		result, err := lifecycle.ExecuteRead(ctx, agentStore, lifecycle.CmdStashPush, func() (struct{}, error) {
			return executeStashPush(ctx, cmd, identity, agentStore, service)
		})
		if err != nil {
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

func executeStashPush(
	ctx context.Context,
	cmd *cli.Command,
	identity *infra.AWSIdentity,
	agentStore store.AgentStore,
	service staging.Service,
) (struct{}, error) {
	existence, err := checkStashExistence(ctx, identity, service)
	if err != nil {
		return struct{}{}, err
	}

	modeResult, err := determineStashPushMode(cmd, service, existence)
	if err != nil {
		return struct{}{}, err
	}

	if modeResult.cancelled {
		return struct{}{}, nil
	}

	passResult, err := getStashPushPassphrase(cmd)
	if err != nil {
		return struct{}{}, err
	}

	if passResult.cancelled {
		return struct{}{}, nil
	}

	fileStore, err := createStashPushFileStore(identity, service, passResult.passphrase)
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
		Encrypted: passResult.passphrase != "",
	}

	return struct{}{}, r.Run(ctx, StashPushOptions{
		Service: service,
		Keep:    cmd.Bool("keep"),
		Mode:    modeResult.mode,
	})
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
