//declscope:package
//
// The core test file is the dialog tests' shared vocabulary: the mutator and
// staging fakes, capability fixtures, key constructors, and drivers below are
// consumed by every dialog's test file, so they are shared package-wide.

//nolint:testpackage // white-box: shared white-box fixtures for the dialog tests
package dialogs

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/hit"
)

// fakeMutator records the last routed write so a dialog's routing can be
// asserted without a real store.
type fakeMutator struct {
	svcCap capability.ServiceCapability

	createCalled  bool
	updateCalled  bool
	deleteCalled  bool
	addTagCalled  bool
	restoreCalled bool

	key            data.StagedKey
	value          string
	typeLabel      string
	description    string
	staged         bool
	recoveryWindow int
	tagKey         string

	outcome data.WriteOutcome

	//declscope:private
	force bool
	//declscope:private
	tagValue string
	//declscope:private
	err error
}

func (m *fakeMutator) Capability() capability.ServiceCapability { return m.svcCap }

func (m *fakeMutator) Create(_ context.Context, key data.StagedKey, value, typeLabel, description string, staged bool) (data.WriteOutcome, error) {
	m.createCalled = true
	m.key, m.value, m.typeLabel, m.description, m.staged = key, value, typeLabel, description, staged

	return m.outcome, m.err
}

func (m *fakeMutator) Update(_ context.Context, key data.StagedKey, value, typeLabel, description string, staged bool) (data.WriteOutcome, error) {
	m.updateCalled = true
	m.key, m.value, m.typeLabel, m.description, m.staged = key, value, typeLabel, description, staged

	return m.outcome, m.err
}

func (m *fakeMutator) Delete(_ context.Context, key data.StagedKey, force bool, recoveryWindow int, staged bool) (data.WriteOutcome, error) {
	m.deleteCalled = true
	m.key, m.force, m.recoveryWindow, m.staged = key, force, recoveryWindow, staged

	return m.outcome, m.err
}

func (m *fakeMutator) AddTag(_ context.Context, key data.StagedKey, tagKey, tagValue string, staged bool) (data.WriteOutcome, error) {
	m.addTagCalled = true
	m.key, m.tagKey, m.tagValue, m.staged = key, tagKey, tagValue, staged

	return m.outcome, m.err
}

func (m *fakeMutator) RemoveTag(_ context.Context, key data.StagedKey, tagKey string, staged bool) (data.WriteOutcome, error) {
	m.key, m.tagKey, m.staged = key, tagKey, staged

	return m.outcome, m.err
}

func (m *fakeMutator) Restore(context.Context, string) (data.WriteOutcome, error) {
	m.restoreCalled = true

	return m.outcome, m.err
}

// Capability fixtures.
func awsParamCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true, HasDescription: true}
}

func appConfigCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true, HasNamespaces: true}
}

func awsSecretCap() capability.ServiceCapability {
	return capability.ServiceCapability{
		Service: "secret", HasTags: true, HasStaging: true, HasRestore: true,
		HasForceDelete: true, HasRecoveryWindow: true, HasDescription: true,
	}
}

func gcloudSecretCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "secret", HasTags: true, HasStaging: true, HasDescription: true}
}

func noStagingParamCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: false}
}

// execCmd runs a command for its side effect on the recording mutator.
func execCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	require.NotNil(t, cmd)

	_ = cmd()
}

// keyMsg builds a printable key press.
func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// keyEnter / keyEsc / keyLeft / keyRight build the newline/discard/choose key
// presses the #790 discard guard and the Stage/Apply popup react to. The Value
// field's ctrl+s ("done") is a huh keymap binding exercised end-to-end by the
// teatest interaction test in the tui package.
func keyEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

func keyEsc() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEscape} }

func keyLeft() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyLeft} }

func keyRight() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyRight} }

// isCanceled reports whether running cmd yields a CanceledMsg (a nil cmd is not).
func isCanceled(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}

	_, ok := cmd().(CanceledMsg)

	return ok
}

// clickAt builds a left click at a hit region's drawn origin, so a dialog mouse
// test derives its coordinate from the layout instead of hard-coding one.
func clickAt(t *testing.T, hits *hit.Map, id string) tea.MouseClickMsg {
	t.Helper()

	x, y, ok := hits.Origin(id)
	require.True(t, ok, "region %q was drawn", id)

	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// stubStaging is a controllable data.StagingService for the apply/reset dialog
// tests: Apply returns a preset result (or a conflict result until conflicts are
// ignored) and records the ignoreConflicts flag each call was made with.
type stubStaging struct {
	service string
	label   string

	// result is returned by Apply when ignoreConflicts is honored (or always,
	// when conflict is empty).
	result data.StagingApplyResult
	// conflict, when non-empty, is returned by Apply while ignoreConflicts is
	// false — modelling a conflict rejection that clears once conflicts are
	// ignored.
	conflict data.StagingApplyResult

	applied []bool

	// resetResult is returned by Reset; resets counts how many times Reset ran
	// (so a fan-out test can assert every target was reset).
	resetResult data.StagingResetResult
	resets      int
}

func (s *stubStaging) Service() string { return s.service }

func (s *stubStaging) Label() string { return s.label }

func (s *stubStaging) Capability() capability.ServiceCapability {
	return capability.ServiceCapability{}
}

func (s *stubStaging) Apply(_ context.Context, ignoreConflicts bool) (data.StagingApplyResult, error) {
	s.applied = append(s.applied, ignoreConflicts)

	if !ignoreConflicts && len(s.conflict.Conflicts) > 0 {
		return s.conflict, nil
	}

	return s.result, nil
}

func (s *stubStaging) Review(context.Context) (data.StagingReview, error) {
	return data.StagingReview{}, nil
}

func (s *stubStaging) Reset(context.Context) (data.StagingResetResult, error) {
	s.resets++

	return s.resetResult, nil
}

func (s *stubStaging) Unstage(context.Context, data.StagedKey) error { return nil }

func (s *stubStaging) CancelAddTag(context.Context, data.StagedKey, string) error { return nil }

func (s *stubStaging) CancelRemoveTag(context.Context, data.StagedKey, string) error {
	return nil
}

// drive runs the dialog's returned command (if any) and feeds its message back,
// returning the updated dialog.
func drive(t *testing.T, d Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return d
	}

	next, _ := d.Update(cmd())

	return next
}

// pressEnter sends an enter key press.
func pressEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

// pressDown sends a down key press.
func pressDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }

// pressPgDown sends a page-down key press (viewport scrolling).
func pressPgDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyPgDown} }

//nolint:gochecknoglobals // test-only type sentinel
var clearScreenType = reflect.TypeOf(tea.ClearScreen())

func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drain(c)...)
		}

		return out
	}

	return []tea.Msg{msg}
}

func hasClearScreen(cmd tea.Cmd) bool {
	for _, m := range drain(cmd) {
		if reflect.TypeOf(m) == clearScreenType {
			return true
		}
	}

	return false
}

// The minimum supported terminal size (#686): every dialog must keep its
// controls and close hint reachable, and wrap long content, at this size.
const (
	minWidth  = 60
	minHeight = 16
)

// ansiSGR matches the SGR color escapes lipgloss emits, so a test can compare
// the plain text a user reads.
var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiSGR.ReplaceAllString(s, "") }

// maxLineWidth is the widest rendered line in display cells (ANSI ignored).
func maxLineWidth(view string) int {
	w := 0
	for line := range strings.SplitSeq(view, "\n") {
		w = max(w, lipgloss.Width(line))
	}

	return w
}

// flatten strips ANSI and per-line trailing padding, then joins the lines with
// no separator, so a value wrapped across lines can be matched as one contiguous
// string (proving it was wrapped, not truncated).
func flatten(view string) string {
	var b strings.Builder

	for line := range strings.SplitSeq(stripANSI(view), "\n") {
		b.WriteString(strings.TrimRight(line, " "))
	}

	return b.String()
}
