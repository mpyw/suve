package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent"
	"github.com/mpyw/suve/internal/staging/file"
)

const (
	// EnvStorage is the environment variable for selecting storage backend.
	EnvStorage = "SUVE_STORAGE"

	// StorageAgent uses the in-memory agent daemon (default).
	StorageAgent = "agent"
	// StorageFile uses the filesystem directly.
	StorageFile = "file"
)

// GetStorageMode returns the current storage mode from environment.
func GetStorageMode() string {
	mode := os.Getenv(EnvStorage)
	if mode == "" {
		return StorageAgent
	}
	return mode
}

// NewStore creates a StoreReadWriter based on the SUVE_STORAGE environment variable.
func NewStore(accountID, region string) (staging.StoreReadWriter, error) {
	switch GetStorageMode() {
	case StorageFile:
		return file.NewStore(accountID, region)
	default:
		// Default to agent storage
		return agent.NewAgentStore(accountID, region), nil
	}
}

// MigrationOptions configures the migration check behavior.
type MigrationOptions struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	AccountID string
	Region    string
}

// CheckAndMigrate checks if there's an existing file state and prompts user to migrate.
// This should be called before creating the agent store for write operations.
// Returns true if migration was performed or not needed, false if user chose to quit.
func CheckAndMigrate(ctx context.Context, opts MigrationOptions) (bool, error) {
	// Only check for agent storage mode
	if GetStorageMode() != StorageAgent {
		return true, nil
	}

	// Check if file state exists
	fileStore, err := file.NewStore(opts.AccountID, opts.Region)
	if err != nil {
		return false, fmt.Errorf("failed to create file store: %w", err)
	}

	// Check if file is encrypted
	isEnc, err := fileStore.IsEncrypted()
	if err != nil {
		return false, fmt.Errorf("failed to check file encryption: %w", err)
	}

	// For encrypted files, we need a passphrase to check contents
	var filePass string
	if isEnc {
		if !terminal.IsTerminalWriter(opts.Stderr) {
			// Non-TTY can't decrypt, skip migration check
			_, _ = fmt.Fprintf(opts.Stderr, "%s Encrypted file detected but cannot prompt for passphrase in non-TTY mode.\n", colors.Warning("!"))
			return true, nil
		}
		prompter := &passphrase.Prompter{
			Stdin:  opts.Stdin,
			Stdout: opts.Stdout,
			Stderr: opts.Stderr,
		}
		_, _ = fmt.Fprintf(opts.Stderr, "%s Encrypted file detected. Passphrase required to check contents.\n", colors.Info("i"))
		filePass, err = prompter.PromptForDecrypt()
		if err != nil {
			return false, fmt.Errorf("failed to get passphrase: %w", err)
		}
	}

	fileState, err := fileStore.LoadWithPassphrase(filePass)
	if err != nil {
		return false, fmt.Errorf("failed to load file state: %w", err)
	}

	// Check if there's anything in the file
	if fileState.IsEmpty() {
		return true, nil
	}

	// Display prompt message per Issue #99 specification
	_, _ = fmt.Fprintln(opts.Stderr, "")
	_, _ = fmt.Fprintf(opts.Stderr, "%s Found existing staged changes in ~/.suve/%s/%s/stage.json\n",
		colors.Warning("!"), opts.AccountID, opts.Region)
	if isEnc {
		_, _ = fmt.Fprintln(opts.Stderr, "This file contains encrypted secrets from a previous session.")
	} else {
		_, _ = fmt.Fprintln(opts.Stderr, "This file contains plain-text secrets from a previous session.")
	}
	_, _ = fmt.Fprintln(opts.Stderr, "")
	_, _ = fmt.Fprintln(opts.Stderr, "Options:")
	_, _ = fmt.Fprintf(opts.Stderr, "  [d] Drain into memory and delete file %s\n", colors.Info("(recommended)"))
	_, _ = fmt.Fprintln(opts.Stderr, "  [k] Keep file and start fresh in memory")
	_, _ = fmt.Fprintln(opts.Stderr, "  [q] Quit")
	_, _ = fmt.Fprintln(opts.Stderr, "")
	_, _ = fmt.Fprintf(opts.Stderr, "%s Choice [d/k/q]: ", colors.Warning("?"))

	// Read user choice
	reader := bufio.NewReader(opts.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	choice := strings.TrimSpace(strings.ToLower(response))

	switch choice {
	case "d", "drain":
		// Drain file into memory
		client := agent.NewClient()
		if err := client.SetState(ctx, opts.AccountID, opts.Region, fileState); err != nil {
			return false, fmt.Errorf("failed to load state into agent: %w", err)
		}

		// Clear file
		emptyState := &staging.State{
			Version: fileState.Version,
			Entries: map[staging.Service]map[string]staging.Entry{
				staging.ServiceParam:  {},
				staging.ServiceSecret: {},
			},
			Tags: map[staging.Service]map[string]staging.TagEntry{
				staging.ServiceParam:  {},
				staging.ServiceSecret: {},
			},
		}
		if err := fileStore.Save(emptyState); err != nil {
			_, _ = fmt.Fprintf(opts.Stderr, "Warning: failed to clear file state: %v\n", err)
		}

		_, _ = fmt.Fprintln(opts.Stdout, "Staged changes loaded into memory. File state cleared.")
		return true, nil

	case "k", "keep":
		// Keep file, start fresh in memory
		_, _ = fmt.Fprintln(opts.Stdout, "Starting fresh in memory. Use SUVE_STORAGE=file to access file storage.")
		return true, nil

	case "q", "quit", "":
		// Quit
		return false, nil

	default:
		_, _ = fmt.Fprintf(opts.Stderr, "Invalid choice: %s. Please enter d, k, or q.\n", choice)
		return false, nil
	}
}
