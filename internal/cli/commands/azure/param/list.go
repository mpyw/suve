package param

import (
	"context"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	genericlist "github.com/mpyw/suve/internal/cli/commands/generic/list"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// namespaceJSONItem is one row of `--output=json` for the namespace-aware
// listing. Namespace is the raw label ("" for the null namespace, matching the
// GUI's per-entry namespace), NOT the "(NULL)" display form used in text.
type namespaceJSONItem struct {
	Namespace string  `json:"namespace"`
	Name      string  `json:"name"`
	Value     *string `json:"value,omitempty"`
}

// ListOptions holds the parsed flags for the App Configuration list command.
type ListOptions struct {
	Prefix string
	Filter string
	Show   bool
	HideNS bool
	Output output.Format
}

// ListRunner renders the Azure App Configuration listing. When Namespace is set
// (the App Configuration default) it prepends a NAMESPACE column; --hide-namespace
// — or the absence of the App-Config extension — falls through to KeyOnly, the
// neutral key-only listing shared with every other provider.
type ListRunner struct {
	// Namespace produces per-(key, namespace) rows for the NAMESPACE column. Nil
	// when the resolved store is not Azure App Configuration.
	Namespace *azure.ListNamespacesUseCase
	// KeyOnly produces the neutral, deduped key-only listing (the --hide-namespace
	// fallback), honoring the store's namespace filter via Reader.List.
	KeyOnly *azure.ListUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Run executes the listing in the mode selected by opts.
func (r *ListRunner) Run(ctx context.Context, opts ListOptions) error {
	if opts.HideNS || r.Namespace == nil {
		return r.runKeyOnly(ctx, opts)
	}

	return r.runNamespaced(ctx, opts)
}

// runKeyOnly reuses the shared generic list renderer so --hide-namespace output
// is byte-for-byte the neutral listing.
func (r *ListRunner) runKeyOnly(ctx context.Context, opts ListOptions) error {
	input := azure.ListInput{Prefix: opts.Prefix, Filter: opts.Filter, WithValue: opts.Show}

	runner := &genericlist.Runner{
		List: func(ctx context.Context) ([]genericlist.Entry, error) {
			result, err := r.KeyOnly.Execute(ctx, input)
			if err != nil {
				return nil, err
			}

			entries := make([]genericlist.Entry, len(result.Entries))
			for i, e := range result.Entries {
				entries[i] = genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
			}

			return entries, nil
		},
		Options: genericlist.Options{Show: opts.Show, Output: opts.Output},
		Stdout:  r.Stdout,
		Stderr:  r.Stderr,
	}

	return runner.Run(ctx)
}

// runNamespaced renders the NAMESPACE column (text: <namespace>TAB<key>[TAB<value>];
// json: {namespace, name, value?}). The null namespace shows as "(NULL)" in text
// but stays "" in JSON so machine consumers see the raw label.
func (r *ListRunner) runNamespaced(ctx context.Context, opts ListOptions) error {
	result, err := r.Namespace.Execute(ctx, azure.ListNamespacesInput{
		Prefix: opts.Prefix, Filter: opts.Filter, WithValue: opts.Show,
	})
	if err != nil {
		return err
	}

	if opts.Output == output.FormatJSON {
		items := make([]namespaceJSONItem, len(result.Entries))
		for i, e := range result.Entries {
			items[i] = namespaceJSONItem{Namespace: e.Namespace, Name: e.Name, Value: e.Value}
		}

		return output.WriteJSON(r.Stdout, items)
	}

	for _, e := range result.Entries {
		ns := e.Namespace
		if ns == "" {
			ns = aznamespace.NullDisplay
		}

		if opts.Show {
			output.Printf(r.Stdout, "%s\t%s\t%s\n", ns, e.Name, lo.FromPtr(e.Value))
		} else {
			output.Printf(r.Stdout, "%s\t%s\n", ns, e.Name)
		}
	}

	return nil
}

// ListCommand returns the Azure App Configuration list command.
//
// Unlike the other providers it does NOT use the generic list scaffold: App
// Configuration keys live in namespaces (the label axis), so by default the
// listing prepends a NAMESPACE column (#430). The default scope is the null
// namespace — matching the GUI's default filter — so every row reads "(NULL)"
// until `--namespace "*"` (or a specific/OR filter) widens it. `--hide-namespace`
// drops the column and falls back to the neutral key-only listing.
func ListCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List settings",
		ArgsUsage: "[filter-prefix]",
		Description: `List settings (key-values) in Azure App Configuration.

Each row is prefixed with the setting's NAMESPACE (the label axis; Azure calls
it a "label"); "(NULL)" is the null/default namespace. The listing is scoped by
--namespace: with no flag it shows the null namespace only (so every row reads
"(NULL)"), "*" shows all namespaces, and "dev,prd"/"dev*" filter by OR/prefix.

Without a filter prefix, lists all keys in the (namespace-)scoped store.
With a filter prefix, lists only keys that start with that prefix.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).

VALUE DISPLAY:
   Use --show to display setting values alongside keys.
   Output format: <namespace><TAB><key><TAB><value>

NAMESPACE COLUMN:
   Use --hide-namespace (--hide-ns) to drop the NAMESPACE column and list keys
   only (the neutral, pipe-friendly output).

EXAMPLES:
   suve azure param list                      List the null namespace
   suve azure param list --namespace '*'      List across all namespaces
   suve azure param list --hide-ns app/       List keys only, no namespace column
   suve azure param list --show app/          List with values
   suve azure param list --output=json app/   List as JSON (namespace field)`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter by regex pattern",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show setting values",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
			&cli.BoolFlag{
				Name:    "hide-namespace",
				Aliases: []string{"hide-ns"},
				Usage:   "Drop the NAMESPACE column and list keys only",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			outputFormat, err := output.ParseFormat(cmd.String("output"))
			if err != nil {
				return err
			}

			store, err := cliinternal.AzureAppConfigStore(ctx)
			if err != nil {
				return err
			}

			runner := &ListRunner{
				KeyOnly: &azure.ListUseCase{Reader: store},
				Stdout:  cmd.Root().Writer,
				Stderr:  cmd.Root().ErrWriter,
			}
			// Only the App Configuration store implements the namespace extension;
			// a store that does not keep the NAMESPACE column off entirely.
			if lister, ok := store.(azure.NamespaceLister); ok {
				runner.Namespace = &azure.ListNamespacesUseCase{Lister: lister}
			}

			return runner.Run(ctx, ListOptions{
				Prefix: cmd.Args().First(),
				Filter: cmd.String("filter"),
				Show:   cmd.Bool("show"),
				HideNS: cmd.Bool("hide-namespace"),
				Output: outputFormat,
			})
		},
	}
}
