package staging

import (
	"github.com/mpyw/suve/internal/tui/data"
)

// rowKind is the type of a selectable staging row.
type rowKind int

const (
	// rowEntry is a staged entry (create/update/delete).
	rowEntry rowKind = iota
	// rowTagAdd is one staged tag add (removable with `u`).
	rowTagAdd
	// rowTagRemove is one staged tag removal (removable with `u`).
	rowTagRemove
)

// rowRef locates a selectable row within a section and carries what an action
// needs to act on it. Every action addresses its item by (section, key) — never
// by a screen coordinate.
type rowRef struct {
	section int
	kind    rowKind

	// entry is the staged entry (rowEntry only).
	entry data.StagedDiffRow

	// key identifies the item (all kinds); tagKey names the tag change the row's
	// cancel action targets (rowTagAdd/rowTagRemove).
	key    data.StagedKey
	tagKey string
}

// rebuildRows flattens every section's entries and tag changes into the
// selectable-row list, clamping the selection into range. It also clears the
// transient invalid-action status: after a reload the row it referred to may no
// longer exist, so the message must not survive (#684).
func (m *Model) rebuildRows() {
	m.status = ""

	var rows []rowRef

	for i, sec := range m.sections {
		for _, e := range sec.entryRows() {
			rows = append(rows, rowRef{
				section: i,
				kind:    rowEntry,
				entry:   e,
				key:     data.StagedKey{Name: e.Name, Namespace: e.Namespace},
			})
		}

		for _, t := range sec.review.Tags {
			key := data.StagedKey{Name: t.Name, Namespace: t.Namespace}

			for _, add := range t.Adds {
				rows = append(rows, rowRef{
					section: i, kind: rowTagAdd, key: key, tagKey: add.Key,
				})
			}

			for _, rem := range t.Removes {
				rows = append(rows, rowRef{
					section: i, kind: rowTagRemove, key: key, tagKey: rem.Key,
				})
			}
		}
	}

	m.rows = rows

	if m.selected >= len(rows) {
		m.selected = max(len(rows)-1, 0)
	}

	if m.selected < 0 {
		m.selected = 0
	}
}

// selectedRow returns the currently-selected row and true, or false when there
// are no rows.
func (m *Model) selectedRow() (rowRef, bool) {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return rowRef{}, false
	}

	return m.rows[m.selected], true
}

// selectedSection returns the index of the section the selection sits in, or 0
// when there is no selection (so a section-scoped action still has a target when
// exactly one section exists).
func (m *Model) selectedSection() int {
	if row, ok := m.selectedRow(); ok {
		return row.section
	}

	return 0
}

// autoUnstaged collects the auto-unstaged keys across all sections for the
// dismissible notice.
func (m *Model) autoUnstaged() []data.StagedKey {
	var keys []data.StagedKey
	for _, sec := range m.sections {
		keys = append(keys, sec.review.AutoUnstaged()...)
	}

	return keys
}

// totalCounts returns the staged entry and tag counts across the given service
// keys (all services when services is nil), for the apply confirmation.
func (m *Model) totalCounts(services map[string]bool) (int, int) {
	entries, tags := 0, 0

	for _, sec := range m.sections {
		if services != nil && !services[sec.service] {
			continue
		}

		entries += sec.review.EntryCount()
		tags += sec.review.TagCount()
	}

	return entries, tags
}
