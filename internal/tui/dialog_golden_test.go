//nolint:testpackage // white-box: hosts the dialogs standalone and shares the vt golden harness
package tui

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/styles"
)

// capMutator is a golden-only Mutator: it carries a capability shape (so a
// dialog gates its controls) and never mutates (goldens render, they do not
// submit).
type capMutator struct{ cap capability.ServiceCapability }

func (m capMutator) Capability() capability.ServiceCapability { return m.cap }
func (capMutator) Create(context.Context, data.StagedKey, string, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (capMutator) Update(context.Context, data.StagedKey, string, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (capMutator) Delete(context.Context, data.StagedKey, bool, int, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (capMutator) AddTag(context.Context, data.StagedKey, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (capMutator) RemoveTag(context.Context, data.StagedKey, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (capMutator) Restore(context.Context, string) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

// hostQuitMsg quits the dialog host without typing into the embedded form.
type hostQuitMsg struct{}

// dialogHost renders a single dialog full-screen (framed like the app overlay)
// so it can be goldened standalone, mirroring the diffHost pattern.
type dialogHost struct {
	m    dialogs.Model
	init tea.Cmd
	st   styles.Styles
	w    int
	h    int
}

func newDialogHost(m dialogs.Model, init tea.Cmd) *dialogHost {
	return &dialogHost{m: m, init: init, st: styles.New()}
}

func (h *dialogHost) Init() tea.Cmd { return h.init }

func (h *dialogHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hostQuitMsg:
		return h, tea.Quit
	case tea.WindowSizeMsg:
		h.w, h.h = msg.Width, msg.Height
	}

	m, cmd := h.m.Update(msg)
	h.m = m

	return h, cmd
}

func (h *dialogHost) View() tea.View {
	v := tea.NewView(h.st.Dialog.Render(h.m.View()))
	v.AltScreen = true

	return v
}

// captureDialog drives a hosted dialog to its rendered state (until marker
// appears) and returns the byte stream, quitting via the host sentinel so no key
// is typed into the embedded form.
func captureDialog(t *testing.T, host *dialogHost, marker string) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, host, teatest.WithInitialTermSize(goldenTermWidth, goldenTermHeight))

	var buf bytes.Buffer

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(&buf, tm.Output())
		if bytes.Contains(buf.Bytes(), []byte(marker)) {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, buf.String(), marker, "dialog content never rendered")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

func dialogGolden(t *testing.T, host *dialogHost, marker string) {
	t.Helper()

	raw := captureDialog(t, host, marker)
	golden.RequireEqual(t, renderVisibleScreen(t, raw))
}

// captureDialogSize is captureDialog at an explicit terminal size, so a dialog
// can be goldened at the minimum supported 60×16 (the #686 clip/wrap fix).
func captureDialogSize(t *testing.T, host *dialogHost, marker string, w, h int) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, host, teatest.WithInitialTermSize(w, h))

	var buf bytes.Buffer

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(&buf, tm.Output())
		if bytes.Contains(buf.Bytes(), []byte(marker)) {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, buf.String(), marker, "dialog content never rendered")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// dialogGoldenSize goldens a hosted dialog rendered at an explicit terminal size
// through the same cell-grid emulator, so the golden captures exactly the
// visible screen a user sees at that size.
func dialogGoldenSize(t *testing.T, host *dialogHost, marker string, w, h int) {
	t.Helper()

	raw := captureDialogSize(t, host, marker, w, h)
	golden.RequireEqual(t, renderVisibleScreenSize(t, raw, w, h))
}

// captureDialogWithKeys drives a hosted dialog, first replaying the given key
// presses (so a golden can capture a post-interaction state — e.g. an immediate
// delete after the mode toggle), then captures once the marker appears. The final
// rendered screen reflects the last frame, so the pre-toggle frame in the stream
// is harmless.
func captureDialogWithKeys(t *testing.T, host *dialogHost, marker string, keys ...tea.KeyPressMsg) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, host, teatest.WithInitialTermSize(goldenTermWidth, goldenTermHeight))

	for _, k := range keys {
		tm.Send(k)
	}

	var buf bytes.Buffer

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(&buf, tm.Output())
		if bytes.Contains(buf.Bytes(), []byte(marker)) {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, buf.String(), marker, "dialog content never rendered")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// keyDownMsg / keyEnterMsg are the golden-driver key presses for navigating a
// custom (non-huh) dialog's control rows.
func keyDownMsg() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func keyEnterMsg() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

func goldenCap(prov, service string) capability.ServiceCapability {
	sc, _ := capabilityFor(provider.Provider(prov), service)

	return sc
}

// TestDialog_EntryFormAWSParamGolden renders the create form for AWS SSM param in
// its default (staged) mode: name, the Type select, value textarea, Description,
// and the mode toggle. The Type select is drawn in both modes — the value type
// flows through the staged path as well as the immediate path (the #664/#680
// fix). No value is seeded, so no secret is rendered.
func TestDialog_EntryFormAWSParamGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "param")},
		Service: "param", Styles: styles.New(),
	})

	dialogGolden(t, newDialogHost(m, cmd), "Value")
}

// TestDialog_EntryFormAppConfigGolden renders the create form for Azure App
// Configuration (namespace field, no Type select).
func TestDialog_EntryFormAppConfigGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("azure", "param")},
		Service: "param", Styles: styles.New(),
	})

	dialogGolden(t, newDialogHost(m, cmd), "Namespace")
}

// TestDialog_EntryFormAWSSecretGolden renders the create form for AWS secret (no
// Type select; mode toggle).
func TestDialog_EntryFormAWSSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "secret")},
		Service: "secret", Styles: styles.New(),
	})

	dialogGolden(t, newDialogHost(m, cmd), "Value")
}

// TestDialog_DeleteAWSSecretGolden renders the delete confirm for AWS secret
// (force + recovery-window rows + mode). The clock is pinned so the
// recoverable-until date is deterministic.
func TestDialog_DeleteAWSSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ + pins the clock
	goldenEnv(t)

	orig := dialogs.NowFunc
	dialogs.NowFunc = func() time.Time { return time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC) }

	t.Cleanup(func() { dialogs.NowFunc = orig })

	m := dialogs.NewDeleteConfirm(dialogs.DeleteInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "secret")},
		Service: "secret", Styles: styles.New(), Name: "prod/api/old-key",
	})

	dialogGolden(t, newDialogHost(m, nil), "Force delete")
}

// TestDialog_DeleteAWSSecretImmediateGolden renders the delete confirm for AWS
// secret AFTER toggling the mode to immediate: the recovery-window row and its
// "recoverable until" line are gone, since an immediate delete cannot carry a
// custom window (GUI parity — SecretDelete(name, force) exposes only force).
func TestDialog_DeleteAWSSecretImmediateGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ + pins the clock
	goldenEnv(t)

	orig := dialogs.NowFunc
	dialogs.NowFunc = func() time.Time { return time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC) }

	t.Cleanup(func() { dialogs.NowFunc = orig })

	m := dialogs.NewDeleteConfirm(dialogs.DeleteInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "secret")},
		Service: "secret", Styles: styles.New(), Name: "prod/api/old-key",
	})

	// From the default (staged) focus on Force, move down to the Mode row and
	// toggle to immediate.
	raw := captureDialogWithKeys(t, newDialogHost(m, nil), "(•) Apply immediately",
		keyDownMsg(), keyDownMsg(), keyEnterMsg())
	golden.RequireEqual(t, renderVisibleScreen(t, raw))
}

// TestDialog_DeleteGCloudSecretGolden renders the delete confirm for Google
// Cloud secret (no force/recovery rows; mode only).
func TestDialog_DeleteGCloudSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m := dialogs.NewDeleteConfirm(dialogs.DeleteInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("googlecloud", "secret")},
		Service: "secret", Styles: styles.New(), Name: "api-key",
	})

	dialogGolden(t, newDialogHost(m, nil), "Delete")
}

// TestDialog_TagAWSParamGolden renders the tag add/remove form.
func TestDialog_TagAWSParamGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewTagForm(dialogs.TagInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "param")},
		Service: "param", Styles: styles.New(), Name: "/app/api/DATABASE_URL",
	})

	dialogGolden(t, newDialogHost(m, cmd), "Action")
}

// TestDialog_EntryFormStagedOnlyGolden renders the edit form as launched from the
// staging review page (StagedOnly): the Stage/Apply-immediately mode toggle is
// gone, since a staged surface offers no immediate-write escape hatch (#679).
func TestDialog_EntryFormStagedOnlyGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "param")},
		Service: "param", Styles: styles.New(),
		Edit: true, Name: "/app/api/DATABASE_URL", Value: "postgres://new", StagedOnly: true,
	})

	dialogGolden(t, newDialogHost(m, cmd), "Value")
}

// TestDialog_TagStagedOnlyGolden renders the tag form as launched from the staging
// review page (StagedOnly): the mode toggle is gone (#679).
func TestDialog_TagStagedOnlyGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewTagForm(dialogs.TagInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "param")},
		Service: "param", Styles: styles.New(), Name: "/app/api/DATABASE_URL", StagedOnly: true,
	})

	dialogGolden(t, newDialogHost(m, cmd), "Action")
}

// TestDialog_ErrorGolden renders the plain error dialog (a blocked operation the
// app surfaces modally). It never mutates and carries no secret.
func TestDialog_ErrorGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m := dialogs.NewError(styles.New(), "Cannot create here",
		"Creating is blocked while viewing all namespaces. Pick one namespace first.")

	dialogGolden(t, newDialogHost(m, nil), "Cannot create here")
}

// minGoldenWidth / minGoldenHeight are the minimum supported terminal size the
// #686 clip/wrap goldens render at.
const (
	minGoldenWidth  = 60
	minGoldenHeight = 16
)

// TestDialog_EntryFormMinSizeGolden pins the #686 fix at the golden layer: at the
// minimum supported 60×16 terminal the AWS secret create form — the tallest
// dialog — caps its body into a scrollable region so the whole box, including
// the submit/cancel hint, stays on-screen instead of clipping off the bottom.
// The rendered screen is the full 60×16 grid, so a clipped hint would be absent.
func TestDialog_EntryFormMinSizeGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "secret")},
		Service: "secret", Styles: styles.New(),
	})

	dialogGoldenSize(t, newDialogHost(m, cmd), "esc: cancel", minGoldenWidth, minGoldenHeight)
}

// TestDialog_ErrorLongMessageMinSizeGolden pins the #686 fix for a long error at
// 60×16: the provider/key-loss message wraps to the dialog width and scrolls
// inside a viewport, so the message body is bounded and the close hint (with the
// scroll affordance) stays pinned on-screen rather than clipping off the bottom.
func TestDialog_ErrorLongMessageMinSizeGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	const msg = "The staging data key could not be recovered from the keychain, so every " +
		"staged change for this scope is unreadable and must be re-created from scratch " +
		"after clearing the store. Check that the login keychain is unlocked, that no other " +
		"process holds it, and that SUVE_STAGING_KEY is either unset or matches the key the " +
		"store was written with. If the keychain entry was deleted, the staged changes " +
		"cannot be recovered and the store must be reset before staging again from this scope. " +
		"Re-run the command after unlocking the keychain to confirm access has been restored, " +
		"then stage the changes again and apply them once the store is readable."

	m := dialogs.NewError(styles.New(), "Staging key lost", msg)

	dialogGoldenSize(t, newDialogHost(m, nil), "enter/esc: close", minGoldenWidth, minGoldenHeight)
}

// TestDialog_RestoreGolden renders the restore form (name input, no mode toggle).
func TestDialog_RestoreGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m, cmd := dialogs.NewRestore(dialogs.RestoreInput{
		Ctx: context.Background(), Mutator: capMutator{cap: goldenCap("aws", "secret")},
		Service: "secret", Styles: styles.New(), Name: "prod/api/deleted",
	})

	dialogGolden(t, newDialogHost(m, cmd), "Restore secret")
}
