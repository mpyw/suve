package cli

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// NewGlobalStashCommand creates a global stash command group that operates on all services.
func NewGlobalStashCommand() *cli.Command {
	return &cli.Command{
		Name:  "stash",
		Usage: "Save staged changes to file for later use",
		Description: `Save staged changes from memory to a file for later restoration.

This command group provides Git-like stash functionality:
   push     Save staged changes to file (default when running 'stash' alone)
   pop      Restore staged changes from file (use --keep to preserve file)
   show     Preview stashed changes without restoring
   drop     Delete stashed changes without restoring

EXAMPLES:
   suve stage stash                    Save changes to file (same as 'stash push')
   suve stage stash push               Save changes to file and clear memory
   suve stage stash push --keep        Save changes to file but keep in memory
   suve stage stash pop                Restore from file and delete file
   suve stage stash pop --keep         Restore from file but keep file
   suve stage stash show               Preview stashed changes
   suve stage stash drop               Delete stashed changes`,
		Action: stashPushAction(""), // Default action = push
		Flags:  stashPushFlags(),    // Default flags = push flags
		Commands: []*cli.Command{
			newGlobalStashPushCommand(),
			newGlobalStashPopCommand(),
			newGlobalStashShowCommand(),
			newGlobalStashDropCommand(),
		},
	}
}

// NewStashCommand creates a service-specific stash command group with the given config.
func NewStashCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "stash",
		Usage: fmt.Sprintf("Save staged %s changes to file for later use", cfg.ItemName),
		Description: fmt.Sprintf(`Save staged %s changes from memory to a file for later restoration.

This command group provides Git-like stash functionality:
   push     Save staged changes to file (default when running 'stash' alone)
   pop      Restore staged changes from file (use --keep to preserve file)
   show     Preview stashed changes without restoring
   drop     Delete stashed changes without restoring

EXAMPLES:
   suve stage %s stash                    Save changes to file
   suve stage %s stash push               Save changes to file and clear memory
   suve stage %s stash pop                Restore from file and delete file
   suve stage %s stash pop --keep         Restore from file but keep file
   suve stage %s stash show               Preview stashed changes
   suve stage %s stash drop               Delete stashed changes`,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Action: stashPushAction(service), // Default action = push
		Flags:  stashPushFlags(),         // Default flags = push flags
		Commands: []*cli.Command{
			newStashPushCommand(cfg),
			newStashPopCommand(cfg),
			newStashShowCommand(cfg),
			newStashDropCommand(cfg),
		},
	}
}

// fileStoreForReading creates a file store for reading operations (service-specific).
// It handles passphrase prompting if the file is encrypted.
// If checkExists is true, returns an error if the file doesn't exist.
func fileStoreForReading(cmd *cli.Command, accountID, region string, service staging.Service, checkExists bool) (*file.Store, error) {
	basicFileStore, err := file.NewStore(accountID, region, service)
	if err != nil {
		return nil, fmt.Errorf("failed to create file store: %w", err)
	}

	if checkExists {
		exists, err := basicFileStore.Exists()
		if err != nil {
			return nil, fmt.Errorf("failed to check stash file: %w", err)
		}

		if !exists {
			return nil, errors.New("no stashed changes")
		}
	}

	isEnc, err := basicFileStore.IsEncrypted()
	if err != nil {
		return nil, fmt.Errorf("failed to check file encryption: %w", err)
	}

	var pass string

	if isEnc {
		prompter := &passphrase.Prompter{
			Stdin:  cmd.Root().Reader,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		switch {
		case cmd.Bool("passphrase-stdin"):
			pass, err = prompter.ReadFromStdin()
			if err != nil {
				return nil, fmt.Errorf("failed to read passphrase from stdin: %w", err)
			}
		case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
			pass, err = prompter.PromptForDecrypt()
			if err != nil {
				return nil, fmt.Errorf("failed to get passphrase: %w", err)
			}
		default:
			return nil, errors.New("encrypted file cannot be decrypted in non-TTY environment; use --passphrase-stdin")
		}
	}

	return file.NewStoreWithPassphrase(accountID, region, service, pass)
}

// fileStoresForReading creates file stores for all services for reading operations (global).
// It handles passphrase prompting if any file is encrypted.
// If checkExists is true, returns an error if no files exist.
func fileStoresForReading(cmd *cli.Command, accountID, region string, checkExists bool) (map[staging.Service]*file.Store, error) {
	basicStores, err := file.NewStoresForAllServices(accountID, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create file stores: %w", err)
	}

	if checkExists {
		anyExists, err := file.AnyExists(basicStores)
		if err != nil {
			return nil, fmt.Errorf("failed to check stash files: %w", err)
		}

		if !anyExists {
			return nil, errors.New("no stashed changes")
		}
	}

	anyEnc, err := file.AnyEncrypted(basicStores)
	if err != nil {
		return nil, fmt.Errorf("failed to check file encryption: %w", err)
	}

	var pass string

	if anyEnc {
		prompter := &passphrase.Prompter{
			Stdin:  cmd.Root().Reader,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		switch {
		case cmd.Bool("passphrase-stdin"):
			pass, err = prompter.ReadFromStdin()
			if err != nil {
				return nil, fmt.Errorf("failed to read passphrase from stdin: %w", err)
			}
		case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
			pass, err = prompter.PromptForDecrypt()
			if err != nil {
				return nil, fmt.Errorf("failed to get passphrase: %w", err)
			}
		default:
			return nil, errors.New("encrypted file cannot be decrypted in non-TTY environment; use --passphrase-stdin")
		}
	}

	return file.NewStoresWithPassphrase(accountID, region, pass)
}
