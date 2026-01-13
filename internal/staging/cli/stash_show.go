package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// StashShowRunner executes stash show operations.
type StashShowRunner struct {
	FileStore *file.Store
	Stdout    io.Writer
	Stderr    io.Writer
}

// StashShowOptions holds options for the stash show command.
type StashShowOptions struct {
	// Service filters the display to a specific service. Empty means all services.
	Service staging.Service
	// Verbose shows detailed information.
	Verbose bool
}

// Run executes the stash show command.
func (r *StashShowRunner) Run(ctx context.Context, opts StashShowOptions) error {
	// Check if file exists
	exists, err := r.FileStore.Exists()
	if err != nil {
		return fmt.Errorf("failed to check stash file: %w", err)
	}

	if !exists {
		return errors.New("no stashed changes")
	}

	// Read state (keep=true to preserve file)
	state, err := r.FileStore.Drain(ctx, "", true)
	if err != nil {
		return fmt.Errorf("failed to read stash file: %w", err)
	}

	// Filter by service if specified
	if opts.Service != "" {
		state = state.ExtractService(opts.Service)
	}

	// Check if there's anything to show
	if state.IsEmpty() {
		if opts.Service != "" {
			return fmt.Errorf("no stashed changes for %s", opts.Service)
		}

		return errors.New("no stashed changes")
	}

	// Display entries
	totalCount := 0
	for service, entries := range state.Entries {
		totalCount += len(entries)
		for name, entry := range entries {
			printer := &staging.EntryPrinter{Writer: r.Stdout}
			printer.PrintEntry(name, entry, opts.Verbose, false)
			output.Printf(r.Stdout, "  Service: %s\n", service)
		}
	}

	// Display tags
	for service, tags := range state.Tags {
		totalCount += len(tags)
		for name, tagEntry := range tags {
			printTagSummary(r.Stdout, name, tagEntry)
			output.Printf(r.Stdout, "  Service: %s\n", service)
		}
	}

	if totalCount == 0 {
		output.Printf(r.Stdout, "No stashed changes\n")
	} else {
		output.Printf(r.Stdout, "\nTotal: %d stashed item(s)\n", totalCount)
	}

	return nil
}

// printTagSummary prints a summary of tag changes.
func printTagSummary(w io.Writer, name string, entry staging.TagEntry) {
	addCount := len(entry.Add)
	removeCount := len(entry.Remove)

	if addCount > 0 && removeCount > 0 {
		output.Printf(w, "  T %s [+%d tags, -%d tags]\n", name, addCount, removeCount)
	} else if addCount > 0 {
		output.Printf(w, "  T %s [+%d tags]\n", name, addCount)
	} else if removeCount > 0 {
		output.Printf(w, "  T %s [-%d tags]\n", name, removeCount)
	}
}

// stashShowFlags returns the common flags for stash show commands.
func stashShowFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "Show detailed information",
		},
		&cli.BoolFlag{
			Name:  "passphrase-stdin",
			Usage: "Read passphrase from stdin (for scripts/automation)",
		},
	}
}

// stashShowAction creates the action function for stash show commands.
func stashShowAction(service staging.Service) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		identity, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return fmt.Errorf("failed to get AWS identity: %w", err)
		}

		fileStore, err := fileStoreForReading(cmd, identity.AccountID, identity.Region, true)
		if err != nil {
			return err
		}

		r := &StashShowRunner{
			FileStore: fileStore,
			Stdout:    cmd.Root().Writer,
			Stderr:    cmd.Root().ErrWriter,
		}

		return r.Run(ctx, StashShowOptions{
			Service: service,
			Verbose: cmd.Bool("verbose"),
		})
	}
}

// newGlobalStashShowCommand creates a global stash show command that operates on all services.
func newGlobalStashShowCommand() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "Preview stashed changes without restoring",
		Description: `Preview the contents of stashed changes without loading them into memory.

This command reads and displays the staging state from the persistent file
without affecting the agent memory or deleting the file.

EXAMPLES:
   suve stage stash show                            Show all stashed changes
   suve stage stash show -v                         Show detailed information
   echo "secret" | suve stage stash show --passphrase-stdin   Decrypt with passphrase`,
		Flags:  stashShowFlags(),
		Action: stashShowAction(""),
	}
}

// newStashShowCommand creates a service-specific stash show command with the given config.
func newStashShowCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:  "show",
		Usage: fmt.Sprintf("Preview stashed %s changes without restoring", cfg.ItemName),
		Description: fmt.Sprintf(`Preview the contents of stashed %s changes without loading them into memory.

This command reads and displays the staging state for %ss from the persistent
file without affecting the agent memory or deleting the file.

EXAMPLES:
   suve stage %s stash show                            Show stashed %s changes
   suve stage %s stash show -v                         Show detailed information`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName),
		Flags:  stashShowFlags(),
		Action: stashShowAction(service),
	}
}
