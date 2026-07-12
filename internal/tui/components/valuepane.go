package components

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/jsonutil"
)

// maskBullet is the character a masked value is rendered with. A masked line is
// a run of these, so a revealed value never reaches the screen (or a golden).
const maskBullet = "•"

// maxMaskWidth caps how many bullets a masked line shows, so a very long secret
// does not paint an enormous bar (and its length is not leaked verbatim).
const maxMaskWidth = 24

// ValuePane renders an entry value in a scrollable viewport, masked by default.
// Reveal is per-pane and never persisted by the pane itself; the owning page
// decides when to reset it (e.g. on selecting another entry). The raw value is
// held privately and only rendered when revealed, so a masked pane never emits
// the real value. A parse-JSON toggle pretty-prints a JSON value (parity with
// the diff page and the GUI), gated behind reveal so a masked secret is never
// normalized.
type ValuePane struct {
	vp        viewport.Model
	raw       string
	secret    bool
	masked    bool
	parseJSON bool
}

// NewValuePane builds an empty, masked value pane.
func NewValuePane() ValuePane {
	return ValuePane{vp: viewport.New(), masked: true}
}

// SetValue loads a value and whether it is secret (and thus masked by default).
// A non-secret value is shown as-is; masking is reset to the secret default and
// the parse-JSON toggle is cleared, so switching entries never carries a
// previous reveal or format toggle forward.
func (p *ValuePane) SetValue(raw string, secret bool) {
	p.raw = raw
	p.secret = secret
	p.masked = secret
	p.parseJSON = false
	p.syncContent()
}

// SetSize sets the pane's inner viewport size.
func (p *ValuePane) SetSize(width, height int) {
	p.vp.SetWidth(max(width, 0))
	p.vp.SetHeight(max(height, 0))
	p.syncContent()
}

// ToggleMask flips masking for a secret value; a non-secret value is never
// masked, so the toggle is a no-op there.
func (p *ValuePane) ToggleMask() {
	if !p.secret {
		return
	}

	p.masked = !p.masked
	p.syncContent()
}

// ToggleParseJSON flips JSON pretty-printing of the value. It is a no-op while
// the value is masked: the mask bullets are not JSON, and normalizing a still-
// hidden secret could leak its structure (the diff page skips parse-json for a
// secret for the same reason), so revealing it first (x) is required.
func (p *ValuePane) ToggleParseJSON() {
	if p.masked {
		return
	}

	p.parseJSON = !p.parseJSON
	p.syncContent()
}

// Masked reports whether the pane is currently masking its value.
func (p *ValuePane) Masked() bool { return p.masked }

// RawValue returns the raw value regardless of masking, for the clipboard copy.
// The copy never changes the mask state, so a masked secret stays masked on
// screen even after it is copied (#689).
func (p *ValuePane) RawValue() string { return p.raw }

// Update forwards a message (e.g. a wheel event) to the viewport for scrolling.
func (p *ValuePane) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd

	p.vp, cmd = p.vp.Update(msg)

	return cmd
}

// HintSuffix is the "(x to reveal)" hint shown next to the Value label for a
// masked secret; empty when there is nothing to reveal.
func (p *ValuePane) HintSuffix() string {
	if p.secret && p.masked {
		return "(x to reveal)"
	}

	return ""
}

// ParseJSONHint is the "(J to format)" / "(J to unformat)" hint shown next to
// the Value label when the visible value is JSON that `J` can pretty-print;
// empty when the value is masked or is not JSON (so the toggle would do
// nothing).
func (p *ValuePane) ParseJSONHint() string {
	if p.masked || !p.isJSON() {
		return ""
	}

	if p.parseJSON {
		return "(J to unformat)"
	}

	return "(J to format)"
}

// isJSON reports whether the raw value is JSON the parse-JSON toggle can format.
func (p *ValuePane) isJSON() bool {
	_, ok := jsonutil.TryFormat(p.raw)

	return ok
}

// View renders the pane body (title is drawn by the owning page).
func (p *ValuePane) View() string {
	return p.vp.View()
}

// syncContent recomputes the viewport content for the current mask state.
func (p *ValuePane) syncContent() {
	p.vp.SetContent(p.display())
}

// display returns the string shown in the viewport: a mask that reveals neither
// the value nor (beyond the cap) its length when masked; the JSON-formatted
// value when revealed with the parse-JSON toggle on and the value is JSON; else
// the raw value.
func (p *ValuePane) display() string {
	if p.masked {
		return MaskValue(p.raw)
	}

	if p.parseJSON {
		if f, ok := jsonutil.TryFormat(p.raw); ok {
			return f
		}
	}

	return p.raw
}

// MaskValue masks a (possibly multi-line) value: each line becomes a run of
// bullets capped at maxMaskWidth, so neither the content nor (beyond the cap)
// the length reaches the screen. Shared by the value pane and the diff page so a
// secret diff is masked identically on both sides.
func MaskValue(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(maskBullet, maskWidth(line))
	}

	return strings.Join(lines, "\n")
}

// maskWidth returns the bullet count for a masked line: the rune length capped
// at maxMaskWidth, and at least one bullet for a non-empty line so an emptyish
// value still reads as "present".
func maskWidth(line string) int {
	n := len([]rune(line))
	if n == 0 {
		return 0
	}

	return min(n, maxMaskWidth)
}
