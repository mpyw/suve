package param

import (
	"context"
	"errors"
	"io"
	"slices"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/generic"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/usecase/param"
)

// namespaceListJSONItem is one row of `--output=json` for the namespace-aware
// listing. Namespace is the raw label ("" for the null namespace, matching the
// GUI's per-entry namespace), NOT the "(NULL)" display form used in text.
type namespaceListJSONItem struct {
	Namespace string  `json:"namespace"`
	Name      string  `json:"name"`
	Value     *string `json:"value,omitempty"`
}

// NamespaceListSource is the App Configuration store's namespace-scoped
// listing, an App-Config-specific extension reached by type-asserting the store.
type NamespaceListSource interface {
	ListWithNamespacesScoped(ctx context.Context) ([]appconfig.KeyNamespace, error)
}

// NewNamespaceLister adapts an App Configuration namespace listing to the use
// case's neutral rows.
func NewNamespaceLister(source NamespaceListSource) param.NamespacesLister {
	return namespaceListAdapter{source: source}
}

// namespaceListAdapter converts App Configuration rows to param.ListNamespacesRow.
type namespaceListAdapter struct {
	source NamespaceListSource
}

func (a namespaceListAdapter) ListNamespaces(ctx context.Context) ([]param.ListNamespacesRow, error) {
	rows, err := a.source.ListWithNamespacesScoped(ctx)
	if err != nil {
		return nil, err
	}

	return lo.Map(rows, func(row appconfig.KeyNamespace, _ int) param.ListNamespacesRow {
		return param.ListNamespacesRow{Key: row.Key, Namespace: row.Namespace, Value: row.Value}
	}), nil
}

// ListOptions holds the parsed flags for the App Configuration list command.
type ListOptions struct {
	Prefix string
	Filter string
	Show   bool
	HideNS bool
	Output output.Format
	// Namespace is the raw --namespace value (the label axis). It is needed only
	// to tell a single literal namespace (per-key Get can fetch values) from a
	// wildcard/OR/prefix one (it cannot — see runKeyOnly).
	Namespace string
}

// ListRunner renders the Azure App Configuration listing. When Namespace is set
// (the App Configuration default) it prepends a NAMESPACE column; --hide-namespace
// — or the absence of the App-Config extension — falls through to KeyOnly, the
// neutral key-only listing shared with every other provider.
type ListRunner struct {
	// Namespace produces per-(key, namespace) rows for the NAMESPACE column. Nil
	// when the resolved store is not Azure App Configuration.
	Namespace *param.ListNamespacesUseCase
	// KeyOnly produces the neutral, deduped key-only listing (the --hide-namespace
	// fallback), honoring the store's namespace filter via Reader.List.
	KeyOnly *param.ListUseCase
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
	runner := &generic.ListRunner{
		List:    r.keyOnlyEntries(opts),
		Options: generic.ListOptions{Show: opts.Show, Output: opts.Output},
		Stdout:  r.Stdout,
		Stderr:  r.Stderr,
	}

	return runner.Run(ctx)
}

// keyOnlyEntries picks how the key-only rows are produced. Values (with --show)
// are normally fetched per key via the neutral use case, but that needs a
// single literal namespace: under a wildcard/OR/prefix --namespace every per-key
// Get fails (Get cannot address all/multiple namespaces), so the whole listing
// would be error rows. In that case source the values from the namespaced list
// — whose response already carries them — and collapse it to key-only rows.
func (r *ListRunner) keyOnlyEntries(opts ListOptions) func(context.Context) ([]generic.ListEntry, error) {
	if opts.Show && r.Namespace != nil {
		if _, err := aznamespace.Literal(opts.Namespace); err != nil {
			return r.keyOnlyEntriesFromNamespaced(opts)
		}
	}

	return r.keyOnlyEntriesFromReader(opts)
}

// keyOnlyEntriesFromReader is the neutral path: keys (and, with --show, values)
// come from the provider-neutral use case, byte-for-byte the shared listing.
func (r *ListRunner) keyOnlyEntriesFromReader(opts ListOptions) func(context.Context) ([]generic.ListEntry, error) {
	return func(ctx context.Context) ([]generic.ListEntry, error) {
		result, err := r.KeyOnly.Execute(ctx, param.ListInput{
			Prefix: opts.Prefix, PlainPrefix: true, Filter: opts.Filter, WithValue: opts.Show,
		})
		if err != nil {
			return nil, err
		}

		entries := lo.Map(result.Entries, func(e param.ListEntry, _ int) generic.ListEntry {
			return generic.ListEntry{Name: e.Name, Value: e.Value, Error: e.Error}
		})

		return entries, nil
	}
}

// keyOnlyEntriesFromNamespaced sources values from the namespaced list (which
// already carries them) and collapses the per-(key, namespace) rows to the
// deduped key-only rows the --hide-namespace listing shows.
func (r *ListRunner) keyOnlyEntriesFromNamespaced(opts ListOptions) func(context.Context) ([]generic.ListEntry, error) {
	return func(ctx context.Context) ([]generic.ListEntry, error) {
		result, err := r.Namespace.Execute(ctx, param.ListNamespacesInput{
			Prefix: opts.Prefix, Filter: opts.Filter, WithValue: true,
		})
		if err != nil {
			return nil, err
		}

		return collapseToKeyOnlyList(result.Entries), nil
	}
}

// collapseToKeyOnlyList reduces per-(key, namespace) rows to the deduped, sorted
// key-only rows the --hide-namespace listing shows, carrying each key's value.
// A key that resolves to different values across namespaces cannot be shown as
// one value, so it becomes an error row rather than an arbitrary pick.
func collapseToKeyOnlyList(rows []param.ListNamespacesEntry) []generic.ListEntry {
	type collapsed struct {
		value     string
		ambiguous bool
	}

	byName := make(map[string]*collapsed, len(rows))

	names := make([]string, 0, len(rows))

	for _, row := range rows {
		value := lo.FromPtr(row.Value)

		if existing, ok := byName[row.Name]; ok {
			if existing.value != value {
				existing.ambiguous = true
			}

			continue
		}

		byName[row.Name] = &collapsed{value: value}
		names = append(names, row.Name)
	}

	slices.Sort(names)

	return lo.Map(names, func(name string, _ int) generic.ListEntry {
		c := byName[name]
		if c.ambiguous {
			return generic.ListEntry{Name: name, Error: errAmbiguousListValue}
		}

		return generic.ListEntry{Name: name, Value: lo.ToPtr(c.value)}
	})
}

// errAmbiguousListValue marks a key whose value differs across the namespaces a
// wildcard --namespace matched, so --hide-namespace cannot show one value.
var errAmbiguousListValue = errors.New("value differs across namespaces; drop --hide-namespace to see each")

// runNamespaced renders the NAMESPACE column (text: <namespace>TAB<key>[TAB<value>];
// json: {namespace, name, value?}). The null namespace shows as "(NULL)" in text
// but stays "" in JSON so machine consumers see the raw label.
func (r *ListRunner) runNamespaced(ctx context.Context, opts ListOptions) error {
	result, err := r.Namespace.Execute(ctx, param.ListNamespacesInput{
		Prefix: opts.Prefix, Filter: opts.Filter, WithValue: opts.Show,
	})
	if err != nil {
		return err
	}

	if opts.Output == output.FormatJSON {
		items := lo.Map(result.Entries, func(e param.ListNamespacesEntry, _ int) namespaceListJSONItem {
			return namespaceListJSONItem{Namespace: e.Namespace, Name: e.Name, Value: e.Value}
		})

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
				KeyOnly: &param.ListUseCase{Reader: store},
				Stdout:  cmd.Root().Writer,
				Stderr:  cmd.Root().ErrWriter,
			}
			// Only the App Configuration store implements the namespace extension;
			// a store that does not keep the NAMESPACE column off entirely.
			if source, ok := store.(NamespaceListSource); ok {
				runner.Namespace = &param.ListNamespacesUseCase{Lister: NewNamespaceLister(source)}
			}

			return runner.Run(ctx, ListOptions{
				Prefix:    cmd.Args().First(),
				Filter:    cmd.String("filter"),
				Show:      cmd.Bool("show"),
				HideNS:    cmd.Bool("hide-namespace"),
				Output:    outputFormat,
				Namespace: cliinternal.AzureAppConfigNamespace(ctx),
			})
		},
	}
}
