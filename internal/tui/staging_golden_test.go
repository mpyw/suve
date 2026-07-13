//nolint:testpackage // white-box: drives the staging page/dialogs over stubs and shares the vt golden harness
package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/styles"
)

// secretStagedValue is the secret staged value the value/diff-view goldens must
// NEVER render revealed.
const secretStagedValue = "super-secret-token-value"

// secureStringParamValue is a SecureString PARAM staged value. It lives in the
// param (non-secret) section, so it proves masking keys off the row's value
// type, not the section's service axis (#677); it must never render revealed.
const secureStringParamValue = "securestring-db-password-value"

// secureStringCreateValue is a SecureString PARAM staged CREATE value. A create
// has no remote to fetch, so its Secret flag is derived from the staged value
// type (#719); it must never render revealed.
const secureStringCreateValue = "securestring-created-api-key-value"

// goldenStaging is a golden-only data.StagingService returning a fixed review
// and (for the results golden) a fixed apply result. It never touches a store.
type goldenStaging struct {
	service     string
	label       string
	svcCap      capability.ServiceCapability
	review      data.StagingReview
	applyResult data.StagingApplyResult
}

func (s *goldenStaging) Service() string                          { return s.service }
func (s *goldenStaging) Label() string                            { return s.label }
func (s *goldenStaging) Capability() capability.ServiceCapability { return s.svcCap }
func (s *goldenStaging) Review(context.Context) (data.StagingReview, error) {
	return s.review, nil
}

func (s *goldenStaging) Apply(context.Context, bool) (data.StagingApplyResult, error) {
	return s.applyResult, nil
}

func (s *goldenStaging) Reset(context.Context) (data.StagingResetResult, error) {
	return data.StagingResetResult{}, nil
}
func (s *goldenStaging) Unstage(context.Context, data.StagedKey) error              { return nil }
func (s *goldenStaging) CancelAddTag(context.Context, data.StagedKey, string) error { return nil }
func (s *goldenStaging) CancelRemoveTag(context.Context, data.StagedKey, string) error {
	return nil
}

// stagingFixture builds the two AWS staged sections: param (an update, a create,
// an auto-unstaged entry, and a tag add + removal) and secret (a delete plus a
// masked-value update).
func stagingFixture() func(string) data.StagingService {
	param := &goldenStaging{
		service: "param", label: "Param", svcCap: goldenCap("aws", "param"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{
				{
					Name: "/app/web/CDN_URL", Type: data.StagedDiffNormal, Operation: "update",
					RemoteValue: "https://cdn-old.example.com", StagedValue: "https://cdn-new.example.com",
				},
				{Name: "/app/web/NEW_FLAG", Type: data.StagedDiffCreate, Operation: "create", StagedValue: "true"},
				{Name: "/app/web/OLD_FLAG", Type: data.StagedDiffAutoUnstaged},
			},
			Tags: []data.StagedTagRow{{
				Name:    "/app/api/DATABASE_URL",
				Adds:    []data.Tag{{Key: "owner", Value: "platform"}},
				Removes: []data.TagRemoval{{Key: "env", Value: "prod"}},
			}},
		},
	}
	secret := &goldenStaging{
		service: "secret", label: "Secret", svcCap: goldenCap("aws", "secret"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{
				{Name: "prod/api/old-key", Type: data.StagedDiffNormal, Operation: "delete", RemoteValue: secretStagedValue},
				{
					Name: "prod/api/session", Type: data.StagedDiffNormal, Operation: "update",
					RemoteValue: "old-" + secretStagedValue, StagedValue: secretStagedValue,
				},
			},
		},
	}

	return func(service string) data.StagingService {
		switch service {
		case "param":
			return param
		case "secret":
			return secret
		default:
			return nil
		}
	}
}

// stagingApp builds an AWS app landed on the Staging tab with the fixture wired.
func stagingApp() *App {
	return newApp(config{
		scope:      provider.Scope{Provider: provider.ProviderAWS},
		service:    "staging",
		identity:   awsIdentityFixture(),
		stagingFor: stagingFixture(),
	})
}

// secureStringStagingFixture wires a param section holding a single SecureString
// param staged update (Secret=true) alongside a plaintext param, so a golden can
// prove the SecureString value is masked in the PARAM (non-secret) section while
// the plaintext one is not — masking keys off the row's value type (#677).
func secureStringStagingFixture() func(string) data.StagingService {
	param := &goldenStaging{
		service: "param", label: "Param", svcCap: goldenCap("aws", "param"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{
				{
					Name: "/app/api/SECURE_TOKEN", Type: data.StagedDiffNormal, Operation: "update", Secret: true,
					RemoteValue: "old-" + secureStringParamValue, StagedValue: secureStringParamValue,
				},
				{
					Name: "/app/web/CDN_URL", Type: data.StagedDiffNormal, Operation: "update",
					RemoteValue: "https://cdn-old.example.com", StagedValue: "https://cdn-new.example.com",
				},
			},
		},
	}

	return func(service string) data.StagingService {
		if service == "param" {
			return param
		}

		return nil
	}
}

// secureStringStagingApp lands on the Staging tab with only the SecureString
// param fixture wired.
func secureStringStagingApp() *App {
	return newApp(config{
		scope:      provider.Scope{Provider: provider.ProviderAWS},
		service:    "staging",
		identity:   awsIdentityFixture(),
		stagingFor: secureStringStagingFixture(),
	})
}

// secureStringCreateStagingFixture wires a param section holding a single
// SecureString param staged CREATE (Secret=true) alongside a plaintext param
// create, so a golden can prove the SecureString create value is masked in the
// PARAM (non-secret) section while the plaintext one is not — a create has no
// remote, so its Secret flag comes from the staged value type (#719).
func secureStringCreateStagingFixture() func(string) data.StagingService {
	param := &goldenStaging{
		service: "param", label: "Param", svcCap: goldenCap("aws", "param"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{
				{
					Name: "/app/api/SECURE_CREATE", Type: data.StagedDiffCreate, Operation: "create", Secret: true,
					StagedValue: secureStringCreateValue,
				},
				{
					Name: "/app/web/NEW_CDN_URL", Type: data.StagedDiffCreate, Operation: "create",
					StagedValue: "https://cdn-new.example.com",
				},
			},
		},
	}

	return func(service string) data.StagingService {
		if service == "param" {
			return param
		}

		return nil
	}
}

// secureStringCreateStagingApp lands on the Staging tab with only the
// SecureString param create fixture wired.
func secureStringCreateStagingApp() *App {
	return newApp(config{
		scope:      provider.Scope{Provider: provider.ProviderAWS},
		service:    "staging",
		identity:   awsIdentityFixture(),
		stagingFor: secureStringCreateStagingFixture(),
	})
}

// TestStaging_SecureStringParamCreateDiffViewGolden pins that a SecureString
// param staged CREATE is masked in the PARAM section, while a plaintext param
// create is shown verbatim — a create derives its Secret flag from the staged
// value type (#719).
func TestStaging_SecureStringParamCreateDiffViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStaging(t, secureStringCreateStagingApp(), "SECURE_CREATE", false)

	assert.NotContains(t, screen, secureStringCreateValue, "no revealed SecureString param create value in the diff-view golden")
	assert.Contains(t, screen, "•", "the SecureString param create row is masked with bullets, proving it renders (not just absent)")
	assert.Contains(t, screen, "https://cdn-new.example.com", "a plaintext param create row is NOT masked")
	golden.RequireEqual(t, screen)
}

// TestStaging_SecureStringParamCreateValueViewGolden pins the same masking after
// toggling to value view.
func TestStaging_SecureStringParamCreateValueViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStaging(t, secureStringCreateStagingApp(), "SECURE_CREATE", true)

	assert.NotContains(t, screen, secureStringCreateValue, "no revealed SecureString param create value in the value-view golden")
	assert.Contains(t, screen, "•", "the SecureString param create row is masked with bullets, proving it renders (not just absent)")
	golden.RequireEqual(t, screen)
}

// TestStaging_SecureStringParamDiffViewGolden pins that a SecureString param
// staged row is masked on both diff sides in the PARAM section, while a
// plaintext param row is shown verbatim — masking keys off the row's value type,
// not the section's service axis (#677).
func TestStaging_SecureStringParamDiffViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStaging(t, secureStringStagingApp(), "SECURE_TOKEN", false)

	assert.Contains(t, screen, secureStringParamValue, "the SecureString param diff is revealed by default (#677/#735)")
	assert.Contains(t, screen, "https://cdn-new.example.com", "a plaintext param row is shown too")
	golden.RequireEqual(t, screen)
}

// TestStaging_SecureStringParamValueViewGolden pins the same masking after
// toggling to value view.
func TestStaging_SecureStringParamValueViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStaging(t, secureStringStagingApp(), "SECURE_TOKEN", true)

	assert.NotContains(t, screen, secureStringParamValue, "no revealed SecureString param value in the value-view golden")
	assert.Contains(t, screen, "•", "the SecureString param row is masked with bullets, proving it renders (not just absent)")
	golden.RequireEqual(t, screen)
}

// TestStaging_DiffViewGolden renders the staging page in the default diff view.
// The remote-vs-staged comparison is a surface the user explicitly opened to
// inspect the change, so secret values are REVEALED by default (#735).
func TestStaging_DiffViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStaging(t, stagingApp(), "prod/api/session", false)

	assert.Contains(t, screen, secretStagedValue, "the secret remote-vs-staged diff is revealed by default (#735)")
	golden.RequireEqual(t, screen)
}

// TestStaging_DiffViewHiddenGolden pins the diff-view mask toggle: pressing `x`
// in diff view hides every secret comparison (page-level), so no secret value
// appears while the rows still render as masked bullet runs.
func TestStaging_DiffViewHiddenGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStagingKeys(t, stagingApp(), "prod/api/session", keyPress('x'))

	assert.NotContains(t, screen, secretStagedValue, "x hides the revealed secret diff")
	assert.Contains(t, screen, "•", "the hidden diff still renders as masked bullets")
	golden.RequireEqual(t, screen)
}

// TestStaging_ValueViewGolden renders the staging page after toggling to value
// view; the secret staged value is masked (never revealed).
func TestStaging_ValueViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStaging(t, stagingApp(), "prod/api/session", true)

	assert.NotContains(t, screen, secretStagedValue, "no revealed secret value in the value-view golden")
	golden.RequireEqual(t, screen)
}

// TestStaging_TagGateStatusGolden pins #684: pressing `t` on a delete-staged
// entry does not open the tag form (a statically impossible transition) but
// shows a one-line status message in the footer instead.
func TestStaging_TagGateStatusGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureStagingKeys(t, stagingApp(), "prod/api/old-key",
		// Move to the delete-staged secret row (prod/api/old-key), then press `t`.
		keyPress('j'), keyPress('j'), keyPress('j'), keyPress('j'), keyPress('t'))

	assert.Contains(t, screen, "cannot tag: staged for deletion", "the gate surfaces a status message")
	golden.RequireEqual(t, screen)
}

// captureStagingKeys drives a staging app to its loaded state, sends the given
// keys, quits, and renders the SETTLED final model's screen.
//
// The golden is taken from tm.FinalModel().View().Content — the full, coherent
// screen of the settled model after every sent message and the quit are
// processed — not from the live teatest frame stream. Bubble Tea emits diff
// frames, and under CI's parallel -race the async load + WindowSizeMsg settle at
// timing-dependent points, so replaying the raw frame stream through the vt
// intermittently corrupts the final screen (dropped separators, uneven padding,
// #764). Rendering the final View().Content is deterministic: one full-width
// paint of the settled state, independent of frame timing.
func captureStagingKeys(t *testing.T, m *App, marker string, presses ...tea.KeyPressMsg) string {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	// Wait for the async staged review to land (the marker row rendered) before
	// sending interaction keys, so the keys act on a loaded page.
	waitFor(t, tm, marker)

	for _, k := range presses {
		tm.Send(k)
	}

	tm.Send(keyPress('q'))

	return settledAppScreen(t, tm)
}

// captureStaging drives a staging app to its loaded state (optionally toggling to
// value view with `v`), quits, and renders the settled final model's screen.
func captureStaging(t *testing.T, m *App, marker string, valueView bool) string {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	waitFor(t, tm, marker)

	if valueView {
		tm.Send(keyPress('v'))
	}

	tm.Send(keyPress('q'))

	return settledAppScreen(t, tm)
}

// settledAppScreen waits for the program to finish, then renders the settled
// *App final model's full-screen View().Content through the vt. Rendering the
// settled View — not the live frame stream — is what makes the golden
// deterministic under CI's parallel -race (#764).
func settledAppScreen(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()

	fm := tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second))

	app, ok := fm.(*App)
	require.True(t, ok, "final model must be *App")

	return renderVisibleScreenSize(t, []byte(app.View().Content), browserTermWidth, browserTermHeight)
}

// waitRenderWidth/waitRenderHeight size the emulator waitFor renders into. They
// are deliberately larger than any test's real terminal (browser 120×34, dialogs
// 100×30) so replaying a stream drawn for the smaller size never re-wraps a marker
// across lines — content is placed at the columns it was drawn to, and the extra
// blank columns/rows are trimmed. The golden itself is still captured at the real
// size from the settled model.
const (
	waitRenderWidth  = 240
	waitRenderHeight = 80
)

// waitFor drains the live output until marker appears in the RENDERED visible
// screen (or the deadline). It matches against the vt-rendered screen — not the
// raw output stream — because CI negotiates incremental cell-updates that can split
// a marker across cursor-positioned writes (e.g. a redraw that keeps a shared
// "Apply " prefix and rewrites only "results"), so a raw bytes.Contains misses text
// the user plainly sees. Rendering through the same emulator the goldens use
// rejoins the cells. It only gates on the async load having rendered; the golden is
// taken from the settled FinalModel, not from this stream.
func waitFor(t *testing.T, tm *teatest.TestModel, marker string) {
	t.Helper()

	var buf bytes.Buffer

	waitForInBuf(t, tm, &buf, marker)
}

// waitForInBuf is waitFor against a caller-owned buffer, so an interaction test
// that gates on several successive markers (initial render → rebuild → popup)
// keeps the full accumulated stream across gates. The vt replay must see the whole
// stream from the capability-handshake preamble on; a fresh buffer per gate would
// replay only the tail (cursor-positioned incremental cell-updates) onto a blank
// grid and miss content drawn earlier. Draining into the SAME buffer keeps every
// gate rendering the true current screen.
func waitForInBuf(t *testing.T, tm *teatest.TestModel, buf *bytes.Buffer, marker string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(buf, tm.Output())
		if buf.Len() > 0 && strings.Contains(renderVisibleScreenSize(t, buf.Bytes(), waitRenderWidth, waitRenderHeight), marker) {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, renderVisibleScreenSize(t, buf.Bytes(), waitRenderWidth, waitRenderHeight), marker, "content never rendered")
}

// TestStaging_ApplyConfirmGolden renders the apply confirmation dialog (target
// identity + ignore-conflicts checkbox), hosted standalone.
func TestStaging_ApplyConfirmGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	d := dialogs.NewApply(dialogs.ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{&goldenStaging{service: "param", label: "Param"}},
		TargetLine: "aws · account 123456789012 · region ap-northeast-1",
		Title:      "Apply staged changes — Param", EntryCount: 2, TagCount: 1, Styles: styles.New(),
	})

	dialogGolden(t, newDialogHost(d, nil), "Ignore conflicts")
}

// TestStaging_ApplyResultsGolden renders the apply results dialog including a
// per-entry failure, a conflict, and a post-apply UnstageError warning.
func TestStaging_ApplyResultsGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	svc := &goldenStaging{
		service: "param", label: "Param",
		applyResult: data.StagingApplyResult{
			ServiceLabel: "Param",
			Entries: []data.ApplyEntryResult{
				{Name: "/app/web/CDN_URL", Status: "updated"},
				{Name: "/app/web/NEW_FLAG", Status: "created", UnstageError: "keychain write failed"},
			},
			Tags:      []data.ApplyTagResult{{Name: "/app/api/DATABASE_URL", Error: "AccessDenied"}},
			Conflicts: []string{"/app/api/REDIS_URL"},
		},
	}

	d := dialogs.NewApply(dialogs.ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc},
		TargetLine: "aws", Title: "Apply staged changes — Param", EntryCount: 2, TagCount: 1, Styles: styles.New(),
	})

	// Focus Apply (row 1) and confirm to reach the results view. Gate on the
	// unstage-warning line — the LAST block the results body renders (entries →
	// tags → conflicts → unstage warnings) — not just the "Apply results" title,
	// so a slow CI apply can never quit on a partially-rendered results frame
	// (#796).
	golden.RequireEqual(t, captureDialogWithKeys(t, newDialogHost(d, nil), "could not be unstaged",
		keyDownMsg(), keyEnterMsg()))
}

// TestStaging_ApplyResultsScrollableGolden pins the #687 fix: an apply result set
// taller than the terminal is capped into a scrollable viewport with the title and
// close hint pinned — so the tail and the hint are never clipped off-screen. The
// 40-entry body cannot fit the 30-row golden terminal, so the frame shows the
// viewport-capped head plus the pinned "scroll · enter/esc: close" hint.
func TestStaging_ApplyResultsScrollableGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	entries := make([]data.ApplyEntryResult, 40)
	for i := range entries {
		entries[i] = data.ApplyEntryResult{Name: fmt.Sprintf("/app/web/PARAM_%02d", i), Status: "updated"}
	}

	svc := &goldenStaging{
		service: "param", label: "Param",
		applyResult: data.StagingApplyResult{ServiceLabel: "Param", Entries: entries},
	}

	// TargetLine is omitted: it appears only on the confirm view, and this golden
	// captures the results view.
	d := dialogs.NewApply(dialogs.ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc},
		Title: "Apply staged changes — Param", EntryCount: len(entries), Styles: styles.New(),
	})

	// Focus Apply (row 1) and confirm to reach the results view.
	golden.RequireEqual(t, captureDialogWithKeys(t, newDialogHost(d, nil), "enter/esc: close",
		keyDownMsg(), keyEnterMsg()))
}

// TestStaging_RoundTrip stages a secret create through the Step-4 mutator, then
// applies it through the staging service over providermock: the section empties
// and its authoritative count drops to zero.
func TestStaging_RoundTrip(t *testing.T) {
	t.Parallel()

	mockStore := testutil.NewMockStore()
	svcCap := goldenCap("aws", "secret")

	prov := &providermock.Store{
		GetFunc: func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound // the secret does not exist yet → a staged create
		},
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, provider.ErrNotFound
		},
		CreateFunc: func(context.Context, string, string, domain.ValueType, string, ...provider.WriteOption) (domain.Version, error) {
			return domain.Version{ID: "1"}, nil
		},
	}

	newStrategy := func(s provider.Store) staging.FullStrategy { return staging.NewAWSSecretStrategy(s) }

	ctx := context.Background()

	// Stage a create through the Step-4 write-path mutator (staged=true).
	mut := data.NewSecretMutator(svcCap, prov, newStrategy, func() (store.ReadWriteOperator, error) {
		return mockStore, nil
	})
	_, err := mut.Create(ctx, data.StagedKey{Name: "prod/api/new-key"}, "v1", "", "", true)
	require.NoError(t, err)

	svc := data.NewStagingService(svcCap, "Secret", func(context.Context) (data.StagingResources, error) {
		return data.StagingResources{Store: mockStore, Strategy: staging.NewAWSSecretStrategy(prov)}, nil
	})

	before, err := svc.Review(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, before.EntryCount(), "the staged create is present before apply")

	res, err := svc.Apply(ctx, false)
	require.NoError(t, err)
	require.Empty(t, res.Conflicts, "no conflict for a create")

	after, err := svc.Review(ctx)
	require.NoError(t, err)
	assert.Zero(t, after.EntryCount()+after.TagCount(), "the section is empty after apply")
}
