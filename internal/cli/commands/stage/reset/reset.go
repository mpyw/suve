// Package reset provides the global reset command for unstaging all changes.
package reset

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
)

// Runner executes the reset command.
type Runner struct {
	Store store.ReadWriteOperator
	// Services lists the provider services in stable display order.
	Services []stgcli.GlobalServiceSpec
	Stdout   io.Writer
	Stderr   io.Writer
}

// Command returns the global reset command for the given provider config.
func Command(cfg stgcli.GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:  "reset",
		Usage: "Unstage all changes",
		Description: `Remove all staged changes from the staging area.

This does not affect the remote store - it only clears the local staging area.

Use 'suve stage <service> reset' for service-specific operations.

EXAMPLES:
   suve stage reset --all    Unstage all changes`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all changes (required)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Require --all flag for safety
			if !cmd.Bool("all") {
				output.Warning(cmd.Root().ErrWriter, "no effect without --all flag")
				output.Hint(cmd.Root().ErrWriter, "Use 'suve stage reset --all' to unstage all changes")

				return nil
			}

			store, _, err := stgcli.WorkingStore(ctx, cfg.ScopeResolver)
			if err != nil {
				return err
			}

			r := &Runner{
				Store:    store,
				Services: cfg.Services,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			return r.Run(ctx)
		},
	}
}

// Run executes the reset command.
func (r *Runner) Run(ctx context.Context) error {
	// Get counts before reset
	staged, err := r.Store.ListEntries(ctx, "")
	if err != nil {
		return err
	}

	tagStaged, err := r.Store.ListTags(ctx, "")
	if err != nil {
		return err
	}

	var (
		totalCount int
		summaries  []string
	)

	for _, spec := range r.Services {
		count := len(staged[spec.Service]) + len(tagStaged[spec.Service])
		totalCount += count
		summaries = append(summaries, formatCount(count, spec.ParserFactory().ServiceName()))
	}

	// Empty service ("") clears all services.
	if err := r.Store.UnstageAll(ctx, ""); err != nil {
		return err
	}

	if totalCount == 0 {
		output.Info(r.Stdout, "No changes staged.")

		return nil
	}

	output.Success(r.Stdout, "Unstaged all changes (%s)", strings.Join(summaries, ", "))

	return nil
}

func formatCount(count int, serviceName string) string {
	return strconv.Itoa(count) + " " + serviceName
}
