package browser

import (
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// rebuildRows recomputes the list rows from the loaded items, the staged-key
// set, and the values/namespace modes.
func (m *Model) rebuildRows() {
	previewW := m.previewWidth()

	rows := lo.Map(m.items, func(it data.Item, _ int) components.ListRow {
		row := components.ListRow{Name: it.Name}

		if m.valuesOn && it.Value != nil {
			row.Preview = previewValue(*it.Value, previewW)
		}

		var badges []string
		if _, staged := m.stagedKeys[dataStagedKey(it)]; staged {
			badges = append(badges, "staged")
		}

		if m.svcCap.HasNamespaces {
			badges = append(badges, namespaceBadge(it.Namespace))
		}

		row.Badges = badges

		return row
	})

	// Show the load-more affordance only when the source reports a real next page.
	// Every provider today lists all names in one shot (NextToken always empty), so
	// the footer stays hidden rather than advertising a phantom "more".
	m.list.SetRows(rows, m.nextToken != "")
}

// previewWidth is the column budget a list value preview may occupy: the list
// pane's inner width (two-pane uses the resizable list width; stacked uses the
// full width) minus the value-line indent EntryList draws. It follows the (now
// variable) list width so a wider list shows a longer preview (#783/#784).
func (m *Model) previewWidth() int {
	listOuter := m.width
	if m.width >= twoPaneMinWidth {
		listOuter = m.listOuterWidth(m.width)
	}

	innerW, _ := components.PaneInner(listOuter, m.height)

	return max(innerW-listPreviewIndent, minPreviewWidth)
}

// minPreviewWidth is the smallest a value preview is truncated to, so a very
// narrow list still shows a few characters rather than an empty line.
const minPreviewWidth = 8

// previewValue renders a list value preview: a single, truncated, newline-
// flattened line fitted to maxWidth columns. It is only ever called under
// values:on — an EXPLICIT reveal, so it SHOWS the real value like the GUI,
// including secrets (mirroring the Compare/diff reveal policy), rather than
// masking it (#734). The detail value pane keeps its own separate mask/reveal.
func previewValue(value string, maxWidth int) string {
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")
	value = replacer.Replace(value)

	if maxWidth < 1 {
		maxWidth = 1
	}

	if len([]rune(value)) > maxWidth {
		return string([]rune(value)[:maxWidth-1]) + "…"
	}

	return value
}

// namespaceBadge renders an item's namespace badge, showing the null namespace
// as "(NULL)".
func namespaceBadge(namespace string) string {
	if namespace == "" {
		return aznamespace.NullDisplay
	}

	return namespace
}

// historyEntries maps neutral history rows onto the history table's presentation
// rows: badges are the state OR the staging labels (never both, never inferred),
// and the tag line is populated only when the provider scopes tags per version.
func historyEntries(_ styles.Styles, rows []data.HistoryRow, tagsPerVersion bool) []components.HistoryEntry {
	return lo.Map(rows, func(r data.HistoryRow, _ int) components.HistoryEntry {
		entry := components.HistoryEntry{
			Label:   r.Label,
			Date:    r.Date,
			Current: r.IsCurrent,
			Badges:  historyBadges(r),
			Value:   r.Value,
			Secret:  r.Secret,
		}

		if tagsPerVersion && len(r.Tags) > 0 {
			entry.TagsLine = "tags: " + tagsInline(r.Tags)
		}

		return entry
	})
}

// historyBadges returns the version's state OR its staging labels — whichever is
// populated — never inferring one axis from the other (#419).
func historyBadges(r data.HistoryRow) []string {
	if len(r.StagingLabels) > 0 {
		return r.StagingLabels
	}

	if r.State != "" {
		return []string{r.State}
	}

	return nil
}

// tagsInline renders tags as "k=v · k2=v2".
func tagsInline(tags []data.Tag) string {
	parts := lo.Map(tags, func(t data.Tag, _ int) string {
		return t.Key + "=" + t.Value
	})

	return strings.Join(parts, " · ")
}

// versionIDs extracts the raw version identifiers in order.
func versionIDs(rows []data.HistoryRow) []string {
	return lo.Map(rows, func(r data.HistoryRow, _ int) string { return r.Version })
}

// namespaceOptions builds the header namespace filter options: the null
// namespace first, the discovered namespaces (excluding the null one, sorted by
// the source), then the all-namespaces "*" option last.
func namespaceOptions(discovered []string) []string {
	opts := []string{""}

	for _, ns := range discovered {
		if ns != "" {
			opts = append(opts, ns)
		}
	}

	return append(opts, aznamespace.AllNamespacesFilter)
}
