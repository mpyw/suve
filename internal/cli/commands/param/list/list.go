// Package list provides the SSM Parameter Store list command.
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
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the list command.
type Runner struct {
	UseCase *param.ListUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the list command.
type Options struct {
	Prefix    string
	Recursive bool
	Filter    string // Regex filter pattern
	Show      bool   // Show parameter values
	Output    output.Format
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
		Usage:     "List parameters",
		ArgsUsage: "[path-prefix]",
		Description: `List parameters in AWS Systems Manager Parameter Store.

Without a path prefix, lists all parameters in the account.
With a path prefix, lists only parameters under that path.

By default, lists only immediate children of the path.
Use --recursive to include all descendant parameters.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).
   The pattern is matched against the full parameter name.

VALUE DISPLAY:
   Use --show to display parameter values alongside names.
   Output format: <name><TAB><value>

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
   suve param list                          List all parameters
   suve param list /app                     List parameters directly under /app
   suve param list --recursive /app         List all parameters under /app recursively
   suve param list /app/config/             List parameters under /app/config
   suve param list --filter '\.prod\.'      List parameters matching regex
   suve param list --show /app              List with values
   suve param list --output=json /app       List as JSON`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "recursive",
				Aliases: []string{"R"},
				Usage:   "List recursively",
			},
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter by regex pattern",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show parameter values",
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
	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	adapter := awsparam.New(client)

	r := &Runner{
		UseCase: &param.ListUseCase{Client: adapter},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Prefix:    cmd.Args().First(),
		Recursive: cmd.Bool("recursive"),
		Filter:    cmd.String("filter"),
		Show:      cmd.Bool("show"),
		Output:    output.ParseFormat(cmd.String("output")),
	})
}

// Run executes the list command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.ListInput{
		Prefix:    opts.Prefix,
		Recursive: opts.Recursive,
		Filter:    opts.Filter,
		WithValue: opts.Show,
	})
	if err != nil {
		return err
	}

	// If --show is not specified and not JSON output, just print names
	if !opts.Show && opts.Output != output.FormatJSON {
		for _, entry := range result.Entries {
			output.Println(r.Stdout, entry.Name)
		}

		return nil
	}

	// For JSON output without --show, output names only
	if opts.Output == output.FormatJSON && !opts.Show {
		items := make([]JSONOutputItem, len(result.Entries))
		for i, entry := range result.Entries {
			items[i] = JSONOutputItem{Name: entry.Name}
		}

		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(items)
	}

	// JSON output with values
	if opts.Output == output.FormatJSON {
		items := make([]JSONOutputItem, 0, len(result.Entries))
		for _, entry := range result.Entries {
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
	for _, entry := range result.Entries {
		if entry.Error != nil {
			output.Printf(r.Stdout, "%s\t<error: %v>\n", entry.Name, entry.Error)
		} else {
			output.Printf(r.Stdout, "%s\t%s\n", entry.Name, lo.FromPtr(entry.Value))
		}
	}

	return nil
}
