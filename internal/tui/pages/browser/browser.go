// Package browser implements the master-detail entry browser shared by the
// Param and Secret tabs: a filterable entry list on the left and, on the right,
// the selected entry's masked value, capability-gated metadata, read-only tags,
// and version history. Compare mode picks two history rows and opens the diff
// page. Every fetch is a tea.Cmd guarded by a monotonic sequence so a stale
// response never overwrites newer state (the GUI's loadSeq pattern), and the
// value and history load independently so a history failure never blanks the
// value.
package browser

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/styles"
)

// Layout constants.
const (
	// twoPaneMinWidth is the width at/above which the list and detail sit side by
	// side; below it they stack.
	twoPaneMinWidth = 110
	// listPaneMaxWidth caps the list pane so a wide terminal gives the detail room.
	listPaneMaxWidth = 46
	// listWidthNum/listWidthDen give the list pane's target width as a fraction of
	// the terminal (2/5), capped by listPaneMaxWidth.
	listWidthNum = 2
	listWidthDen = 5
	// stackedMinPaneHeight is the smallest a stacked pane may shrink to.
	stackedMinPaneHeight = 3
	// paneContentTop is the page-local row a pane's content starts on relative to
	// the pane top: one border row plus one title row.
	paneContentTop = 2
	// paneBorderLeft is the pane's left border column.
	paneBorderLeft = 1
	// valuePaneHeight is the detail value pane's fixed line count.
	valuePaneHeight = 3
	// debounceDelay is how long a prefix/filter edit waits before reloading, so a
	// burst of keystrokes issues one list load (the last-write-wins sequence guard
	// drops the rest).
	debounceDelay = 250 * time.Millisecond
)

// focus selects which widget the navigation keys drive.
type focus int

const (
	focusList focus = iota
	focusHistory
	focusPrefix
	focusFilter
)

// Page-local key bindings not present in the global map.
//
//nolint:gochecknoglobals // immutable page-local bindings
var (
	prefixKey    = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prefix"))
	filterKey    = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	valuesKey    = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "values"))
	recursiveKey = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "recursive/refresh"))
	loadMoreKey  = key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "load more"))
	revealKey    = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "reveal"))
	parseJSONKey = key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "parse-json"))
	compareKey   = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compare"))
	stagingKey   = key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "staging"))
	spaceKey     = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "pick/namespace"))

	// Mutation keys: open the create/edit/delete/tag/restore dialogs.
	newKey     = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new"))
	editKey    = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	deleteKey  = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))
	tagKey     = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tag"))
	restoreKey = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore"))
)

// Model is the browser page.
type Model struct {
	// ctx is the Run context threaded through every fetch command, so a fetch is
	// cancelled when the program exits.
	ctx context.Context //nolint:containedctx // fetch commands need the Run context; mirrors the GUI

	source  data.Source
	staging data.StagingProbe // may be nil (no staging badges)
	svcCap  capability.ServiceCapability

	styles styles.Styles
	keys   keys.Map

	width  int
	height int

	// Header state.
	prefix    textinput.Model
	filter    textinput.Model
	valuesOn  bool
	recursive bool
	focus     focus

	// App Configuration namespace filter.
	namespaces []string // discovered, plus the null/all options
	nsIndex    int

	// List state.
	list       components.EntryList
	items      []data.Item
	nextToken  string
	stagedKeys map[data.StagedKey]struct{}
	// deleteStagedKeys is the subset of stagedKeys staged for deletion; the
	// edit/delete/tag affordances are dead-end transitions on such an entry (the
	// reducer rejects them), so they are gated on this set (#692).
	deleteStagedKeys map[data.StagedKey]struct{}
	// entryStagedKeys and tagStagedKeys split stagedKeys by change kind (value/entry
	// vs tag), so the detail banner distinguishes value-only / tag-only / both,
	// matching the GUI's StagingBanner (#701).
	entryStagedKeys map[data.StagedKey]struct{}
	tagStagedKeys   map[data.StagedKey]struct{}
	loading         bool
	spinner         spinner.Model

	// Detail state.
	valuePane       components.ValuePane
	history         components.HistoryTable
	historyVersions []string // raw version ids in display order (maps picks → diff)
	detail          data.Detail
	detailOK        bool

	// Error state is split per source (mirroring the staging page's per-section
	// err and the GUI's per-source error fields) so a transient detail/history
	// failure clears the moment that source next succeeds and never lingers over
	// another source's correct data. listErr/detailErr/historyErr track the three
	// content loads; stagedErr holds the launch-time staging-store hard-fail, which
	// is a persistent condition and is not cleared by a selection change.
	listErr    string
	detailErr  string
	historyErr string
	stagedErr  string
	// actionStatus is a transient one-line message shown when a key is pressed on
	// a row where its transition is a dead-end (e.g. edit/delete/tag on a
	// delete-staged entry) — the browser's parity with the staging page's #684
	// invalid-action status. It is cleared on the next key press.
	actionStatus string

	// Monotonic sequence guards (GUI loadSeq pattern): a response is applied only
	// when its seq still matches the latest issued one.
	listSeq     int
	detailSeq   int
	historySeq  int
	stagedSeq   int
	nsSeq       int
	debounceSeq int

	// hits is the last-rendered hit map: one compositor region per clickable/
	// scrollable area (header fields and toggles, the list, the detail pane, the
	// value label, and the history band), rebuilt every View so a mouse coordinate
	// is hit-tested against the layers rather than a hand-maintained geometry.
	hits *hit.Map
	// regions is the scratch slice View appends region layers to before building
	// hits, reused across frames to avoid per-frame allocation churn.
	regions []*lipgloss.Layer
}

// Clickable/scrollable region IDs for the browser hit map. The list and history
// bands map a click to a row (via the in-region offset); the detail and value
// regions route the wheel to the value pane; the header regions each reduce a
// click to the same action their key equivalent performs.
const (
	regionList       = "list"
	regionDetail     = "detail"
	regionHistory    = "history"
	regionValueLabel = "value-label"
	regionNamespace  = "ns"
	regionPrefix     = "prefix"
	regionFilter     = "filter"
	regionValues     = "values"
	regionRecursive  = "recursive"
	regionRefresh    = "refresh"
)

// New builds a browser page over a data source. ctx is the Run context threaded
// through every fetch. staging may be nil when the service has no staging
// workflow.
func New(ctx context.Context, source data.Source, staging data.StagingProbe, st styles.Styles, km keys.Map) *Model {
	prefix := textinput.New()
	prefix.Prompt = ""

	filter := textinput.New()
	filter.Prompt = ""

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	m := &Model{
		ctx:              ctx,
		source:           source,
		staging:          staging,
		svcCap:           source.Capability(),
		styles:           st,
		keys:             km,
		prefix:           prefix,
		filter:           filter,
		spinner:          sp,
		list:             components.NewEntryList(st),
		history:          components.NewHistoryTable(st),
		valuePane:        components.NewValuePane(),
		stagedKeys:       map[data.StagedKey]struct{}{},
		deleteStagedKeys: map[data.StagedKey]struct{}{},
		entryStagedKeys:  map[data.StagedKey]struct{}{},
		tagStagedKeys:    map[data.StagedKey]struct{}{},
		// Recursive listing defaults on (GUI parity): a param browser shows the whole
		// subtree under a prefix by default. The toggle is only shown/effective for a
		// non-namespaced param service; elsewhere the field is inert.
		recursive: true,
	}
	if m.svcCap.HasNamespaces {
		// Seed the namespace filter with the null and all-namespaces options; the
		// discovered namespaces are inserted between them once loaded.
		m.namespaces = []string{"", aznamespace.AllNamespacesFilter}
	}

	return m
}

// Init dispatches the initial loads: list, staged flags, and (App Config)
// discovered namespaces.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadListCmd(false), m.spinner.Tick}
	if m.staging != nil {
		cmds = append(cmds, m.loadStagedCmd())
	}

	if m.svcCap.HasNamespaces {
		cmds = append(cmds, m.loadNamespacesCmd())
	}

	return tea.Batch(cmds...)
}
