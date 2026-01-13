// Package list provides the Secrets Manager list command.
package list

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the list command.
type Runner struct {
	UseCase *secret.ListUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the list command.
type Options struct {
	Prefix string
	Filter string // Regex filter pattern
	Show   bool   // Show secret values
	Output output.Format
}

// JSONOutputItem represents a single item in JSON output.
type JSONOutputItem struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"` // nil when error or not requested, pointer to distinguish from empty string
	Error string  `json:"error,omitempty"`
}

// Command returns the list command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List secrets",
		ArgsUsage: "[filter-prefix]",
		Description: `List secrets in AWS Secrets Manager.

Without a filter prefix, lists all secrets in the account.
With a filter prefix, lists only secrets whose names contain that prefix.

Note: Unlike SSM parameters, Secrets Manager filters by name substring,
not by path hierarchy.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).
   The pattern is matched against the full secret name.

VALUE DISPLAY:
   Use --show to display secret values alongside names.
   Output format: <name><TAB><value>

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
   suve secret list                       List all secrets
   suve secret list prod                  List secrets containing "prod"
   suve secret list my-app/               List secrets starting with "my-app/"
   suve secret list --filter '\.prod$'    List secrets matching regex
   suve secret list --show prod           List with values
   suve secret list --output=json prod    List as JSON`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter by regex pattern",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show secret values",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		UseCase: &secret.ListUseCase{Client: client},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Prefix: cmd.Args().First(),
		Filter: cmd.String("filter"),
		Show:   cmd.Bool("show"),
		Output: output.ParseFormat(cmd.String("output")),
	})
}

// Run executes the list command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.ListInput{
		Prefix:    opts.Prefix,
		Filter:    opts.Filter,
		WithValue: opts.Show,
	})
	if err != nil {
		return err
	}

	entries := result.Entries

	// If --show is not specified and not JSON output, just print names
	if !opts.Show && opts.Output != output.FormatJSON {
		for _, entry := range entries {
			output.Println(r.Stdout, entry.Name)
		}

		return nil
	}

	// For JSON output without --show, output names only
	if opts.Output == output.FormatJSON && !opts.Show {
		items := make([]JSONOutputItem, len(entries))
		for i, entry := range entries {
			items[i] = JSONOutputItem{Name: entry.Name}
		}

		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(items)
	}

	// JSON output with values
	if opts.Output == output.FormatJSON {
		items := make([]JSONOutputItem, 0, len(entries))
		for _, entry := range entries {
			if entry.Error != nil {
				items = append(items, JSONOutputItem{Name: entry.Name, Error: entry.Error.Error()})
			} else {
				items = append(items, JSONOutputItem{Name: entry.Name, Value: entry.Value})
			}
		}

		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(items)
	}

	// Text output with values
	for _, entry := range entries {
		if entry.Error != nil {
			output.Printf(r.Stdout, "%s\t<error: %v>\n", entry.Name, entry.Error)
		} else {
			output.Printf(r.Stdout, "%s\t%s\n", entry.Name, lo.FromPtr(entry.Value))
		}
	}

	return nil
}
