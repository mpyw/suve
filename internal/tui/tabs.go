//declscope:namespace app
//
// Tab layout and tab switching for the App's tab bar.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/keys"
)

// Service tab keys: the two capability services plus the synthetic staging tab
// (which is not a capability service of its own but a workflow over the offered
// ones).
const (
	serviceParam   = "param"
	serviceSecret  = "secret"
	stagingService = "staging"
)

// buildTabs derives the tab bar for a launched scope from the neutral
// capability matrix. It filters capability.All() to the scope's provider, gates
// each service on scope presence (Azure Key Vault needs a vault name, App
// Configuration needs a store name — via provider.Scope.SupportsService), and
// appends a Staging tab when any offered service supports staging.
func buildTabs(scope provider.Scope) []components.Tab {
	var (
		tabs       []components.Tab
		hasStaging bool
	)

	for _, pc := range capability.All() {
		if pc.Provider != string(scope.Provider) {
			continue
		}

		for _, sc := range pc.Services {
			if !serviceAvailable(scope, sc.Service) {
				continue
			}

			tabs = append(tabs, components.Tab{Title: sc.DisplayName, Service: sc.Service})

			if sc.HasStaging {
				hasStaging = true
			}
		}
	}

	if hasStaging {
		tabs = append(tabs, components.Tab{Title: "Staging", Service: stagingService})
	}

	return tabs
}

// serviceAvailable reports whether a capability service is reachable for the
// scope, applying Azure's per-service scope gating through the neutral
// SupportsService seam. Non-Azure providers list only services they support, so
// this is a no-op for them.
func serviceAvailable(scope provider.Scope, service string) bool {
	switch service {
	case serviceParam:
		return scope.SupportsService(provider.KindParam)
	case serviceSecret:
		return scope.SupportsService(provider.KindSecret)
	default:
		return true
	}
}

// initialTabIndex returns the index of the tab matching the launch service
// (from `suve azure secret --tui` etc.), or 0 when the service is empty or not
// present among the tabs.
func initialTabIndex(tabs []components.Tab, service string) int {
	if service == "" {
		return 0
	}

	for i, t := range tabs {
		if t.Service == service {
			return i
		}
	}

	return 0
}

// numberedTabJump maps a 1/2/3 key press to its zero-based tab index. The index
// comes from the binding's position, so there are no magic tab numbers.
func numberedTabJump(k keys.Map, msg tea.KeyPressMsg) (int, bool) {
	for i, binding := range []key.Binding{k.Tab1, k.Tab2, k.Tab3} {
		if key.Matches(msg, binding) {
			return i, true
		}
	}

	return 0, false
}

// cycleTab moves the active tab by delta, wrapping around the ends, and returns
// the new page's Init command.
func (m *App) cycleTab(delta int) tea.Cmd {
	n := len(m.tabs)
	if n == 0 {
		return nil
	}

	return m.setTab(((m.activeTab+delta)%n + n) % n)
}

// jumpTab selects tab i directly, ignoring an index past the last tab (so "3"
// with two tabs is a no-op rather than snapping to the last), and returns the
// new page's Init command.
func (m *App) jumpTab(i int) tea.Cmd {
	if i < 0 || i >= len(m.tabs) {
		return nil
	}

	return m.setTab(i)
}

// setTab switches to a valid tab index, swaps in that tab's page (resetting any
// pushed sub-page), and returns the page's Init command.
func (m *App) setTab(i int) tea.Cmd {
	if i == m.activeTab {
		return nil
	}

	m.activeTab = i

	p, cmd := m.pageForTab(i)
	m.pages = []page{p}
	m.forwardResizeToTop()

	return cmd
}

// pageForTab builds the page for a tab index: the staging page for the Staging
// tab (when its seam is wired), a browser page for a param/secret service (when
// a data source is wired), else the placeholder.
func (m *App) pageForTab(i int) (page, tea.Cmd) {
	tab := m.tabs[i]

	if tab.Service == stagingService {
		if services := m.stagingServicesFor(m.offeredServices()); len(services) > 0 {
			p := newStagingPage(m.runCtx, services, m.styles, m.keys)

			return p, p.Init()
		}

		return newPlaceholderPage(m.styles, tab.Title, placeholderNotice(tab)), nil
	}

	if m.sourceFor != nil {
		if source, staging := m.sourceFor(tab.Service); source != nil {
			m.pageGen++
			p := newBrowserPage(m.runCtx, m.pageGen, source, staging, m.scope.AppConfigNamespace, m.styles, m.keys)

			return p, p.Init()
		}
	}

	return newPlaceholderPage(m.styles, tab.Title, placeholderNotice(tab)), nil
}

// offeredServices returns the service-axis keys the scope offers (the non-staging
// tabs, in tab order), so the staging page and the apply/reset fan-out iterate
// exactly the services that have a browser tab.
func (m *App) offeredServices() []string {
	var services []string

	for _, t := range m.tabs {
		if t.Service != stagingService {
			services = append(services, t.Service)
		}
	}

	return services
}
