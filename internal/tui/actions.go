//declscope:namespace app
//
// The actions Update dispatches for page requests: the mutation dialogs
// (entry, delete, tag, restore) and the staging apply, reset and detail flows.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/nav"
)

// onMutationDone closes the dialog, voices the outcome, and reloads the active
// browser page (list/detail/staged badges) so the mutation is reflected.
func (m *App) onMutationDone(msg dialogs.MutationDoneMsg) tea.Cmd {
	m.popDialog()
	m.status = msg.Status

	return m.reloadActivePage()
}

// reloadActivePage asks the active page to reload after a mutation. A browser
// page reloads its list, detail, and staged badges; other pages ignore it.
func (m *App) reloadActivePage() tea.Cmd {
	if len(m.pages) == 0 {
		return nil
	}

	top := len(m.pages) - 1
	p, cmd := m.pages[top].Update(nav.Reload{})
	m.pages[top] = p

	return cmd
}

// openEntryForm builds and pushes the create/edit dialog for a service.
func (m *App) openEntryForm(req nav.OpenEntryForm) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles,
		Edit: req.Edit, Name: req.Name, Namespace: req.Namespace,
		Value: req.Value, TypeLabel: req.TypeLabel, Description: req.Description,
		StagedOnly: req.StagedOnly, DeleteStagedKeys: req.DeleteStagedKeys,
	})

	return m.pushDialog(d, cmd)
}

// openDelete builds and pushes the delete-confirm dialog for a service.
func (m *App) openDelete(req nav.OpenDelete) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d := dialogs.NewDeleteConfirm(dialogs.DeleteInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles,
		Name: req.Name, Namespace: req.Namespace,
	})

	return m.pushDialog(d, nil)
}

// openTag builds and pushes the tag add/remove dialog for a service.
func (m *App) openTag(req nav.OpenTag) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d, cmd := dialogs.NewTagForm(dialogs.TagInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles,
		Name: req.Name, Namespace: req.Namespace, Tags: req.Tags, StagedOnly: req.StagedOnly,
	})

	return m.pushDialog(d, cmd)
}

// openRestore builds and pushes the restore dialog for a service.
func (m *App) openRestore(req nav.OpenRestore) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d, cmd := dialogs.NewRestore(dialogs.RestoreInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles, Name: req.Name,
	})

	return m.pushDialog(d, cmd)
}

// mutatorForService resolves the write seam for a service, or nil when none is
// wired (an uninitialized shell, or a service with no mutator).
func (m *App) mutatorForService(service string) data.Mutator {
	if m.mutatorFor == nil {
		return nil
	}

	return m.mutatorFor(service)
}

// refreshStagingTab updates the Staging tab's title with the current staged
// total ("Staging" at zero, "Staging(n)" otherwise).
func (m *App) refreshStagingTab() {
	total := 0
	for _, c := range m.stagedCounts {
		total += c
	}

	for i, t := range m.tabs {
		if t.Service == stagingService {
			title := "Staging"
			if total > 0 {
				title += "(" + strconv.Itoa(total) + ")"
			}

			m.tabs[i].Title = title

			return
		}
	}
}

// stagingServicesFor resolves the staging seams for a set of service keys,
// dropping any the factory does not offer (nil seam), preserving order.
func (m *App) stagingServicesFor(services []string) []data.StagingService {
	if m.stagingFor == nil {
		return nil
	}

	out := make([]data.StagingService, 0, len(services))

	for _, s := range services {
		if svc := m.stagingFor(s); svc != nil {
			out = append(out, svc)
		}
	}

	return out
}

// openApply builds and pushes the apply confirmation for the requested services.
func (m *App) openApply(req nav.OpenApply) tea.Cmd {
	targets := m.stagingServicesFor(req.Services)
	if len(targets) == 0 {
		return nil
	}

	d := dialogs.NewApply(dialogs.ApplyInput{
		Ctx: m.runCtx, Targets: targets, TargetLine: m.applyTargetLine(),
		Title: applyTitle(req.Global, targets), EntryCount: req.EntryCount, TagCount: req.TagCount,
		Styles: m.styles,
	})

	return m.pushDialog(d, nil)
}

// openReset builds and pushes the reset confirmation for the requested services.
func (m *App) openReset(req nav.OpenReset) tea.Cmd {
	targets := m.stagingServicesFor(req.Services)
	if len(targets) == 0 {
		return nil
	}

	d := dialogs.NewReset(dialogs.ResetInput{
		Ctx: m.runCtx, Targets: targets, Title: resetTitle(req.Global, targets), Styles: m.styles,
	})

	return m.pushDialog(d, nil)
}

// pushStagingDetail pushes a full remote-vs-staged diff page for the staging
// page's `enter` detail, reusing the diff viewer over static content.
func (m *App) pushStagingDetail(req nav.OpenStagingDetail) tea.Cmd {
	p := newStaticDiffPage(data.DiffContent{
		OldLabel: req.OldLabel, NewLabel: req.NewLabel,
		OldValue: req.OldValue, NewValue: req.NewValue, Secret: req.Secret,
	}, m.styles, m.keys)
	m.pages = append(m.pages, p)
	m.forwardResizeToTop()

	return p.Init()
}

// applyTargetLine renders the apply target (the provider's display name plus
// its resolved target segments) shown on the apply confirmation — parity with the CLI's
// prompt.
func (m *App) applyTargetLine() string {
	parts := []string{capability.DisplayName(m.scope.Provider)}
	if target := m.target.String(); target != "" {
		parts = append(parts, target)
	}

	return strings.Join(parts, " · ")
}

// applyTitle names the apply confirmation: "— all" for the fan-out, else the
// single service's label.
func applyTitle(global bool, targets []data.StagingService) string {
	return "Apply staged changes — " + targetTitle(global, targets)
}

// resetTitle names the reset confirmation.
func resetTitle(global bool, targets []data.StagingService) string {
	return "Reset staged changes — " + targetTitle(global, targets)
}

// targetTitle is "all" for a global fan-out, else the single target's label.
func targetTitle(global bool, targets []data.StagingService) string {
	if global || len(targets) != 1 {
		return "all"
	}

	return targets[0].Label()
}
