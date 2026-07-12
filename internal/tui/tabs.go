package tui

import (
	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
)

// stagingService is the synthetic tab service key for the staging page (which
// is not a capability service of its own but a workflow over the offered ones).
const stagingService = "staging"

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
	case "param":
		return scope.SupportsService(provider.KindParam)
	case "secret":
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
