// Package drain provides the stage drain command.
package drain

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent"
	"github.com/mpyw/suve/internal/staging/file"
)

// Command returns the stage drain command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "drain",
		Usage: "Load staged changes from file into memory",
		Description: `Load staged changes from file into the in-memory agent.

This command loads the staging state from the persistent file storage
(~/.suve/{accountID}/{region}/stage.json) into the agent daemon.

By default, the file is deleted after loading.
Use --keep to retain the file after draining.

If the agent already has staged changes, you'll be prompted to confirm
the action. Use --force to skip the prompt and overwrite, or --merge
to merge the file changes with existing memory changes.

EXAMPLES:
   suve stage drain                            Load from file and delete file
   suve stage drain --keep                     Load from file and keep file
   suve stage drain --force                    Overwrite agent memory without prompt
   suve stage drain --merge                    Merge file with existing memory
   echo "secret" | suve stage drain --passphrase-stdin   Decrypt with passphrase from stdin`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "keep",
				Usage: "Keep the file after loading into memory",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite agent memory without prompt",
			},
			&cli.BoolFlag{
				Name:  "merge",
				Usage: "Merge file changes with existing memory",
			},
			&cli.BoolFlag{
				Name:  "passphrase-stdin",
				Usage: "Read passphrase from stdin (for scripts/automation)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			identity, err := infra.GetAWSIdentity(ctx)
			if err != nil {
				return fmt.Errorf("failed to get AWS identity: %w", err)
			}

			// Load from file store
			fileStore, err := file.NewStore(identity.AccountID, identity.Region)
			if err != nil {
				return fmt.Errorf("failed to create file store: %w", err)
			}

			// Check if file is encrypted
			isEnc, err := fileStore.IsEncrypted()
			if err != nil {
				return fmt.Errorf("failed to check file encryption: %w", err)
			}

			var pass string
			if isEnc {
				prompter := &passphrase.Prompter{
					Stdin:  cmd.Root().Reader,
					Stdout: cmd.Root().Writer,
					Stderr: cmd.Root().ErrWriter,
				}
				if cmd.Bool("passphrase-stdin") {
					pass, err = prompter.ReadFromStdin()
					if err != nil {
						return fmt.Errorf("failed to read passphrase from stdin: %w", err)
					}
				} else if terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
					pass, err = prompter.PromptForDecrypt()
					if err != nil {
						return fmt.Errorf("failed to get passphrase: %w", err)
					}
				} else {
					return errors.New("encrypted file cannot be decrypted in non-TTY environment; use --passphrase-stdin")
				}
			}

			fileState, err := fileStore.LoadWithPassphrase(pass)
			if err != nil {
				return fmt.Errorf("failed to load state from file: %w", err)
			}

			// Check if there's anything to drain
			if fileState.IsEmpty() {
				return errors.New("no staged changes in file to drain")
			}

			client := agent.NewClient()
			force := cmd.Bool("force")
			merge := cmd.Bool("merge")

			// Check if agent already has staged changes
			agentState, err := client.GetState(ctx, identity.AccountID, identity.Region)
			if err != nil {
				// Agent might not be running, which is fine
				agentState = nil
			}

			if !agentState.IsEmpty() && !force && !merge {
				return errors.New("agent already has staged changes; use --force to overwrite or --merge to combine")
			}

			var finalState *staging.State
			if merge && !agentState.IsEmpty() {
				// Merge states
				finalState = mergeStates(agentState, fileState)
			} else {
				// Use file state directly
				finalState = fileState
			}

			// Set state in agent
			if err := client.SetState(ctx, identity.AccountID, identity.Region, finalState); err != nil {
				return fmt.Errorf("failed to set state in agent: %w", err)
			}

			// Delete file unless --keep is specified
			if !cmd.Bool("keep") {
				// Clear the file by saving empty state
				emptyState := &staging.State{
					Version: finalState.Version,
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
					_, _ = fmt.Fprintf(cmd.Root().ErrWriter, "Warning: failed to delete file: %v\n", err)
				}
				_, _ = fmt.Fprintln(cmd.Root().Writer, "Staged changes loaded from file and file deleted")
			} else {
				_, _ = fmt.Fprintln(cmd.Root().Writer, "Staged changes loaded from file (file kept)")
			}

			return nil
		},
	}
}

// mergeStates merges two states, with fileState taking precedence for conflicts.
func mergeStates(agentState, fileState *staging.State) *staging.State {
	version := 2 // Default version
	if fileState != nil {
		version = fileState.Version
	}
	result := &staging.State{
		Version: version,
		Entries: make(map[staging.Service]map[string]staging.Entry),
		Tags:    make(map[staging.Service]map[string]staging.TagEntry),
	}

	// Initialize maps
	for _, svc := range []staging.Service{staging.ServiceParam, staging.ServiceSecret} {
		result.Entries[svc] = make(map[string]staging.Entry)
		result.Tags[svc] = make(map[string]staging.TagEntry)
	}

	// Copy agent state first
	if agentState != nil {
		for svc, entries := range agentState.Entries {
			for name, entry := range entries {
				result.Entries[svc][name] = entry
			}
		}
		for svc, tags := range agentState.Tags {
			for name, tag := range tags {
				result.Tags[svc][name] = tag
			}
		}
	}

	// Overlay file state (takes precedence)
	if fileState != nil {
		for svc, entries := range fileState.Entries {
			for name, entry := range entries {
				result.Entries[svc][name] = entry
			}
		}
		for svc, tags := range fileState.Tags {
			for name, tag := range tags {
				result.Tags[svc][name] = tag
			}
		}
	}

	return result
}
