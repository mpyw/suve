// Package list provides the SSM Parameter Store list command.
package list

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
)

// Client is the interface for the list command.
type Client interface {
	paramapi.DescribeParametersAPI
	paramapi.GetParameterAPI
}

// Runner executes the list command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
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

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
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
	// Compile regex filter if specified
	var filterRegex *regexp.Regexp
	if opts.Filter != "" {
		var err error
		filterRegex, err = regexp.Compile(opts.Filter)
		if err != nil {
			return fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	option := "OneLevel"
	if opts.Recursive {
		option = "Recursive"
	}

	input := &paramapi.DescribeParametersInput{}
	if opts.Prefix != "" {
		input.ParameterFilters = []paramapi.ParameterStringFilter{
			{
				Key:    lo.ToPtr("Path"),
				Option: lo.ToPtr(option),
				Values: []string{opts.Prefix},
			},
		}
	}

	// Collect all parameter names first
	var names []string
	paginator := paramapi.NewDescribeParametersPaginator(r.Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to describe parameters: %w", err)
		}

		for _, param := range page.Parameters {
			name := lo.FromPtr(param.Name)
			// Apply regex filter if specified
			if filterRegex != nil && !filterRegex.MatchString(name) {
				continue
			}
			names = append(names, name)
		}
	}

	// If --show is not specified and not JSON output, just print names
	if !opts.Show && opts.Output != output.FormatJSON {
		for _, name := range names {
			_, _ = fmt.Fprintln(r.Stdout, name)
		}
		return nil
	}

	// For JSON output without --show, output names only
	if opts.Output == output.FormatJSON && !opts.Show {
		items := make([]JSONOutputItem, len(names))
		for i, name := range names {
			items[i] = JSONOutputItem{Name: name}
		}
		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Fetch values in parallel
	namesMap := make(map[string]struct{}, len(names))
	for _, name := range names {
		namesMap[name] = struct{}{}
	}

	results := parallel.ExecuteMap(ctx, namesMap, func(ctx context.Context, name string, _ struct{}) (string, error) {
		out, err := r.Client.GetParameter(ctx, &paramapi.GetParameterInput{
			Name:           lo.ToPtr(name),
			WithDecryption: lo.ToPtr(true),
		})
		if err != nil {
			return "", err
		}
		return lo.FromPtr(out.Parameter.Value), nil
	})

	// JSON output with values
	if opts.Output == output.FormatJSON {
		items := make([]JSONOutputItem, 0, len(names))
		for _, name := range names {
			result := results[name]
			if result.Err != nil {
				items = append(items, JSONOutputItem{Name: name, Error: result.Err.Error()})
			} else {
				items = append(items, JSONOutputItem{Name: name, Value: lo.ToPtr(result.Value)})
			}
		}
		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Text output with values
	for _, name := range names {
		result := results[name]
		if result.Err != nil {
			_, _ = fmt.Fprintf(r.Stdout, "%s\t<error: %v>\n", name, result.Err)
		} else {
			_, _ = fmt.Fprintf(r.Stdout, "%s\t%s\n", name, result.Value)
		}
	}

	return nil
}
