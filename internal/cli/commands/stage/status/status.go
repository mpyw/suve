// Package status provides the global status command for viewing all staged changes.
package status

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
)

// Runner executes the status command.
type Runner struct {
	Store  *staging.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the status command.
type Options struct {
	Verbose bool
}

// Command returns the status command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show all staged changes (SSM Parameter Store and Secrets Manager)",
		Description: `Display all staged changes for both SSM Parameter Store and Secrets Manager.

Use -v/--verbose to show detailed information including the staged values.

EXAMPLES:
   suve stage status     Show all staged changes
   suve stage status -v  Show detailed information`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	identity, err := infra.GetAWSIdentity(ctx)
	if err != nil {
		return fmt.Errorf("failed to get AWS identity: %w", err)
	}
	store, err := staging.NewStore(identity.AccountID, identity.Region)
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	opts := Options{
		Verbose: cmd.Bool("verbose"),
	}

	return r.Run(ctx, opts)
}

// Run executes the status command.
func (r *Runner) Run(_ context.Context, opts Options) error {
	entries, err := r.Store.ListEntries("")
	if err != nil {
		return err
	}

	tagEntries, err := r.Store.ListTags("")
	if err != nil {
		return err
	}

	// Check if there are any changes
	hasChanges := false
	for _, serviceEntries := range entries {
		if len(serviceEntries) > 0 {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		for _, serviceTags := range tagEntries {
			if len(serviceTags) > 0 {
				hasChanges = true
				break
			}
		}
	}

	if !hasChanges {
		_, _ = fmt.Fprintln(r.Stdout, "No changes staged.")
		return nil
	}

	printer := &staging.EntryPrinter{Writer: r.Stdout}

	// Show SSM Parameter Store changes (no DeleteOptions for SSM Parameter Store)
	paramEntries := entries[staging.ServiceParam]
	paramTagEntries := tagEntries[staging.ServiceParam]
	paramTotal := len(paramEntries) + len(paramTagEntries)
	if paramTotal > 0 {
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning("Staged SSM Parameter Store changes"), paramTotal)
		printEntries(printer, paramEntries, opts.Verbose, false)
		printTagEntries(r.Stdout, paramTagEntries, opts.Verbose)
	}

	// Show Secrets Manager changes (with DeleteOptions)
	secretEntries := entries[staging.ServiceSecret]
	secretTagEntries := tagEntries[staging.ServiceSecret]
	secretTotal := len(secretEntries) + len(secretTagEntries)
	if secretTotal > 0 {
		// Add spacing if we printed SSM Parameter Store entries
		if paramTotal > 0 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		_, _ = fmt.Fprintf(r.Stdout, "%s (%d):\n", colors.Warning("Staged Secrets Manager changes"), secretTotal)
		printEntries(printer, secretEntries, opts.Verbose, true)
		printTagEntries(r.Stdout, secretTagEntries, opts.Verbose)
	}

	return nil
}

func printEntries(printer *staging.EntryPrinter, entries map[string]staging.Entry, verbose, showDeleteOptions bool) {
	// Sort names for consistent output
	for _, name := range maputil.SortedKeys(entries) {
		printer.PrintEntry(name, entries[name], verbose, showDeleteOptions)
	}
}

func printTagEntries(w io.Writer, tagEntries map[string]staging.TagEntry, verbose bool) {
	for _, name := range maputil.SortedKeys(tagEntries) {
		entry := tagEntries[name]
		parts := []string{}
		if len(entry.Add) > 0 {
			parts = append(parts, fmt.Sprintf("+%d tag(s)", len(entry.Add)))
		}
		if entry.Remove.Len() > 0 {
			parts = append(parts, fmt.Sprintf("-%d tag(s)", entry.Remove.Len()))
		}
		summary := strings.Join(parts, ", ")
		_, _ = fmt.Fprintf(w, "  %s %s [%s]\n", colors.Info("T"), name, summary)

		if verbose {
			for key, value := range entry.Add {
				_, _ = fmt.Fprintf(w, "      + %s=%s\n", key, value)
			}
			for key := range entry.Remove {
				_, _ = fmt.Fprintf(w, "      - %s\n", key)
			}
		}
	}
}
