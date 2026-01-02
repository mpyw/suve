// Package list provides the Secrets Manager list command.
package list

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/parallel"
)

// Client is the interface for the list command.
type Client interface {
	secretapi.ListSecretsAPI
	secretapi.GetSecretValueAPI
}

// Runner executes the list command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
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
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
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
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
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
	// Compile regex filter if specified
	var filterRegex *regexp.Regexp
	if opts.Filter != "" {
		var err error
		filterRegex, err = regexp.Compile(opts.Filter)
		if err != nil {
			return fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	input := &secretapi.ListSecretsInput{}
	if opts.Prefix != "" {
		input.Filters = []secretapi.Filter{
			{
				Key:    secretapi.FilterNameStringTypeName,
				Values: []string{opts.Prefix},
			},
		}
	}

	// Collect all secret names first
	var names []string
	paginator := secretapi.NewListSecretsPaginator(r.Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, secret := range page.SecretList {
			name := lo.FromPtr(secret.Name)
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
		out, err := r.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
			SecretId: lo.ToPtr(name),
		})
		if err != nil {
			return "", err
		}
		return lo.FromPtr(out.SecretString), nil
	})

	// JSON output with values
	if opts.Output == output.FormatJSON {
		items := make([]JSONOutputItem, 0, len(names))
		for _, name := range names {
			result := results[name]
			if result.Err == nil {
				items = append(items, JSONOutputItem{Name: name, Value: result.Value})
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
