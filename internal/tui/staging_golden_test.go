//nolint:testpackage // white-box: drives the staging page/dialogs over stubs and shares the vt golden harness
package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// TestStaging_DiffViewGolden renders the staging page in the default diff view.
// Secret values are masked on both diff sides, so the secret value never appears.
func TestStaging_DiffViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	raw := captureStaging(t, stagingApp(), "prod/api/session", false)
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	assert.NotContains(t, screen, secretStagedValue, "no revealed secret value in the diff-view golden")
	golden.RequireEqual(t, screen)
}

// TestStaging_ValueViewGolden renders the staging page after toggling to value
// view; the secret staged value is masked (never revealed).
func TestStaging_ValueViewGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	raw := captureStaging(t, stagingApp(), "prod/api/session", true)
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	assert.NotContains(t, screen, secretStagedValue, "no revealed secret value in the value-view golden")
	golden.RequireEqual(t, screen)
}

// TestStaging_TagGateStatusGolden pins #684: pressing `t` on a delete-staged
// entry does not open the tag form (a statically impossible transition) but
// shows a one-line status message in the footer instead.
func TestStaging_TagGateStatusGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	raw := captureStagingKeys(t, stagingApp(), "prod/api/old-key",
		// Move to the delete-staged secret row (prod/api/old-key), then press `t`.
		keyPress('j'), keyPress('j'), keyPress('j'), keyPress('j'), keyPress('t'))
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	assert.Contains(t, screen, "cannot tag: staged for deletion", "the gate surfaces a status message")
	golden.RequireEqual(t, screen)
}

// captureStagingKeys drives a staging app to its loaded state, sends the given
// keys, and returns the captured byte stream (for asserting a post-key frame).
func captureStagingKeys(t *testing.T, m *App, marker string, presses ...tea.KeyPressMsg) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	var buf bytes.Buffer

	waitFor(t, tm, &buf, marker)

	for _, k := range presses {
		tm.Send(k)
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)

	_, _ = io.Copy(&buf, tm.Output())

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// captureStaging drives a staging app to its loaded state (optionally toggling to
// value view with `v`) and returns the captured byte stream.
func captureStaging(t *testing.T, m *App, marker string, valueView bool) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	var buf bytes.Buffer

	waitFor(t, tm, &buf, marker)

	if valueView {
		tm.Send(keyPress('v'))
		time.Sleep(100 * time.Millisecond)

		_, _ = io.Copy(&buf, tm.Output())
	}

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// waitFor accumulates output until marker appears (or the deadline).
func waitFor(t *testing.T, tm *teatest.TestModel, buf *bytes.Buffer, marker string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(buf, tm.Output())
		if bytes.Contains(buf.Bytes(), []byte(marker)) {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, buf.String(), marker, "staging content never rendered")
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

	// Focus Apply (row 1) and confirm to reach the results view.
	raw := captureDialogWithKeys(t, newDialogHost(d, nil), "Apply results",
		keyDownMsg(), keyEnterMsg())
	golden.RequireEqual(t, renderVisibleScreen(t, raw))
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
	raw := captureDialogWithKeys(t, newDialogHost(d, nil), "enter/esc: close",
		keyDownMsg(), keyEnterMsg())
	golden.RequireEqual(t, renderVisibleScreen(t, raw))
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
