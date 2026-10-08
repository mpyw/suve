package param

import (
	"context"
	"errors"
	"io"
	"slices"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/namespaces"
	"github.com/mpyw/suve/internal/usecase/param"
)

// NamespaceListSource is the App Configuration store's namespace-scoped
// listing, an App-Config-specific extension reached by type-asserting the store.
type NamespaceListSource interface {
	ListWithNamespacesScoped(ctx context.Context) ([]appconfig.KeyNamespace, error)
}

// namespaceListJSONItem is one row of `--output=json` for the namespace-aware
// listing. Namespace is the raw label ("" for the null namespace, matching the
// GUI's per-entry namespace), NOT the "(NULL)" display form used in text.
type namespaceListJSONItem struct {
	Namespace string  `json:"namespace"`
	Name      string  `json:"name"`
	Value     *string `json:"value,omitempty"`
}

// newNamespaceLister adapts an App Configuration namespace listing to the use
// case's neutral rows.
func newNamespaceLister(source NamespaceListSource) param.NamespacesLister {
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

// listOptions holds the parsed flags for the App Configuration list command.
type listOptions struct {
	prefix string
	filter string
	show   bool
	hideNS bool
	output output.Format
	// namespace is the raw --namespace value (the label axis). It is needed only
	// to tell a single literal namespace (per-key Get can fetch values) from a
	// wildcard/OR/prefix one (it cannot — see runKeyOnly).
	namespace string
}

// listRunner renders the Azure App Configuration listing. When Namespace is set
// (the App Configuration default) it prepends a NAMESPACE column; --hide-namespace
// — or the absence of the App-Config extension — falls through to keyOnly, the
// neutral key-only listing shared with every other provider.
type listRunner struct {
	// namespace produces per-(key, namespace) rows for the NAMESPACE column. Nil
	// when the resolved store is not Azure App Configuration.
	namespace *param.ListNamespacesUseCase
	// keyOnly produces the neutral, deduped key-only listing (the --hide-namespace
	// fallback), honoring the store's namespace filter via Reader.List.
	keyOnly *param.ListUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// run executes the listing in the mode selected by opts.
func (r *listRunner) run(ctx context.Context, opts listOptions) error {
	if opts.hideNS || r.namespace == nil {
		return r.runKeyOnly(ctx, opts)
	}

	return r.runNamespaced(ctx, opts)
}

// runKeyOnly reuses the shared generic list renderer so --hide-namespace output
// is byte-for-byte the neutral listing.
func (r *listRunner) runKeyOnly(ctx context.Context, opts listOptions) error {
	runner := &generic.ListRunner{
		List:    r.keyOnlyEntries(opts),
		Options: generic.ListOptions{Show: opts.show, Output: opts.output},
		Stdout:  r.stdout,
		Stderr:  r.stderr,
	}

	return runner.Run(ctx)
}

// keyOnlyEntries picks how the key-only rows are produced. Values (with --show)
// are normally fetched per key via the neutral use case, but that needs a
// single literal namespace: under a wildcard/OR/prefix --namespace every per-key
// Get fails (Get cannot address all/multiple namespaces), so the whole listing
// would be error rows. In that case source the values from the namespaced list
// — whose response already carries them — and collapse it to key-only rows.
func (r *listRunner) keyOnlyEntries(opts listOptions) func(context.Context) ([]generic.ListEntry, error) {
	if opts.show && r.namespace != nil {
		if _, err := namespaces.Literal(opts.namespace); err != nil {
			return r.keyOnlyEntriesFromNamespaced(opts)
		}
	}

	return r.keyOnlyEntriesFromReader(opts)
}

// keyOnlyEntriesFromReader is the neutral path: keys (and, with --show, values)
// come from the provider-neutral use case, byte-for-byte the shared listing.
func (r *listRunner) keyOnlyEntriesFromReader(opts listOptions) func(context.Context) ([]generic.ListEntry, error) {
	return func(ctx context.Context) ([]generic.ListEntry, error) {
		result, err := r.keyOnly.Execute(ctx, param.ListInput{
			Prefix: opts.prefix, PlainPrefix: true, Filter: opts.filter, WithValue: opts.show,
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
func (r *listRunner) keyOnlyEntriesFromNamespaced(opts listOptions) func(context.Context) ([]generic.ListEntry, error) {
	return func(ctx context.Context) ([]generic.ListEntry, error) {
		result, err := r.namespace.Execute(ctx, param.ListNamespacesInput{
			Prefix: opts.prefix, Filter: opts.filter, WithValue: true,
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

		return generic.ListEntry{Name: name, Value: new(c.value)}
	})
}

// errAmbiguousListValue marks a key whose value differs across the namespaces a
// wildcard --namespace matched, so --hide-namespace cannot show one value.
var errAmbiguousListValue = errors.New("value differs across namespaces; drop --hide-namespace to see each")

// runNamespaced renders the NAMESPACE column (text: <namespace>TAB<key>[TAB<value>];
// json: {namespace, name, value?}). The null namespace shows as "(NULL)" in text
// but stays "" in JSON so machine consumers see the raw label.
func (r *listRunner) runNamespaced(ctx context.Context, opts listOptions) error {
	result, err := r.namespace.Execute(ctx, param.ListNamespacesInput{
		Prefix: opts.prefix, Filter: opts.filter, WithValue: opts.show,
	})
	if err != nil {
		return err
	}

	if opts.output == output.FormatJSON {
		items := lo.Map(result.Entries, func(e param.ListNamespacesEntry, _ int) namespaceListJSONItem {
			return namespaceListJSONItem{Namespace: e.Namespace, Name: e.Name, Value: e.Value}
		})

		return output.WriteJSON(r.stdout, items)
	}

	for _, e := range result.Entries {
		ns := e.Namespace
		if ns == "" {
			ns = namespaces.NullDisplay
		}

		if opts.show {
			output.Printf(r.stdout, "%s\t%s\t%s\n", ns, e.Name, lo.FromPtr(e.Value))
		} else {
			output.Printf(r.stdout, "%s\t%s\n", ns, e.Name)
		}
	}

	return nil
}

// listCommand returns the Azure App Configuration list command.
//
// Unlike the other providers it does NOT use the generic list scaffold: App
// Configuration keys live in namespaces (the label axis), so by default the
// listing prepends a NAMESPACE column (#430). The default scope is the null
// namespace — matching the GUI's default filter — so every row reads "(NULL)"
// until `--namespace "*"` (or a specific/OR filter) widens it. `--hide-namespace`
// drops the column and falls back to the neutral key-only listing.
//
//declscope:shared // command.go registers it
func listCommand() *cli.Command {
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

			store, err := azureinternal.AppConfigStore(ctx)
			if err != nil {
				return err
			}

			runner := &listRunner{
				keyOnly: &param.ListUseCase{Reader: store},
				stdout:  cmd.Root().Writer,
				stderr:  cmd.Root().ErrWriter,
			}
			// Only the App Configuration store implements the namespace extension;
			// a store that does not keep the NAMESPACE column off entirely.
			if source, ok := store.(NamespaceListSource); ok {
				runner.namespace = &param.ListNamespacesUseCase{Lister: newNamespaceLister(source)}
			}

			return runner.run(ctx, listOptions{
				prefix:    cmd.Args().First(),
				filter:    cmd.String("filter"),
				show:      cmd.Bool("show"),
				hideNS:    cmd.Bool("hide-namespace"),
				output:    outputFormat,
				namespace: azureinternal.AppConfigNamespace(ctx),
			})
		},
	}
}
