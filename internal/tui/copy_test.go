//nolint:testpackage // white-box: exercises the app's copy wiring and the setClipboard seam
package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
)

// fakeCopyPage is a page that supplies a copy value (and records that CopyText
// revealed it), so the app-level `y` wiring can be asserted without a real
// browser page or an async load.
type fakeCopyPage struct {
	text     string
	revealed bool
}

func (p *fakeCopyPage) Update(tea.Msg) (page, tea.Cmd) { return p, nil }
func (p *fakeCopyPage) View(int, int) string           { return "" }

func (p *fakeCopyPage) CopyText() (string, bool) {
	p.revealed = true
	if p.text == "" {
		return "", false
	}

	return p.text, true
}

// TestApp_CopyWritesActivePageValue pins that `y` copies the active page's
// revealed value through the OSC52 seam (asserted via a stub, never real escape
// bytes), and that an empty value is a guarded no-op so it never clears the
// clipboard.
//
//nolint:paralleltest // swaps the package-level setClipboard seam; must not race other tests
func TestApp_CopyWritesActivePageValue(t *testing.T) {
	copied := ""
	called := false
	orig := setClipboard
	setClipboard = func(s string) tea.Cmd {
		called = true
		copied = s

		return nil
	}

	t.Cleanup(func() { setClipboard = orig })

	app := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	fp := &fakeCopyPage{text: "s3cr3t"}
	app.pages = []page{fp}

	_ = updateApp(t, app, keyPress('y'))

	assert.True(t, called, "y copies the active page's value")
	assert.Equal(t, "s3cr3t", copied)
	assert.True(t, fp.revealed, "CopyText reveals before returning — never copies a masked value")

	// An empty value must not reach the clipboard (an OSC52 with "" clears it).
	called = false
	empty := &fakeCopyPage{text: ""}
	app.pages = []page{empty}

	_ = updateApp(t, app, keyPress('y'))
	assert.False(t, called, "an empty copy is a guarded no-op")
}
