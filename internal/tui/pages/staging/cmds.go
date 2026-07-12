package staging

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
)

// Async result messages. Each staged read carries the section index and the
// sequence its fetch was issued with, so the reducer drops a stale response.
type (
	reviewLoadedMsg struct {
		section int
		seq     int
		review  data.StagingReview
		err     error
	}
	// actionDoneMsg reports an inline row action (unstage / cancel-tag) finished;
	// the page reloads on success or surfaces the error.
	actionDoneMsg struct {
		section int
		err     error
	}
)

// unstageCmd runs an unstage (entry + tags) for a row's item off the update loop.
func (m *Model) unstageCmd(sectionIdx int, key data.StagedKey) tea.Cmd {
	ctx := m.ctx
	svc := m.sections[sectionIdx].svc

	return func() tea.Msg {
		return actionDoneMsg{section: sectionIdx, err: svc.Unstage(ctx, key)}
	}
}

// cancelAddTagCmd cancels one staged tag add.
func (m *Model) cancelAddTagCmd(sectionIdx int, key data.StagedKey, tagKey string) tea.Cmd {
	ctx := m.ctx
	svc := m.sections[sectionIdx].svc

	return func() tea.Msg {
		return actionDoneMsg{section: sectionIdx, err: svc.CancelAddTag(ctx, key, tagKey)}
	}
}

// cancelRemoveTagCmd cancels one staged tag removal.
func (m *Model) cancelRemoveTagCmd(sectionIdx int, key data.StagedKey, tagKey string) tea.Cmd {
	ctx := m.ctx
	svc := m.sections[sectionIdx].svc

	return func() tea.Msg {
		return actionDoneMsg{section: sectionIdx, err: svc.CancelRemoveTag(ctx, key, tagKey)}
	}
}
