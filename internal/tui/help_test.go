//nolint:testpackage // white-box: exercises the app's composed help key map and the pages' PageKeyMaps
package tui

import (
	"testing"

	"charm.land/bubbles/v2/help"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

// shortDescs returns the descriptions of a key map's short-help bindings, so a
// test can assert on the visible labels without depending on their order or the
// raw key glyphs.
func shortDescs(km help.KeyMap) []string {
	descs := make([]string, 0, len(km.ShortHelp()))
	for _, b := range km.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}

	return descs
}

// shortKeys returns the key glyphs of a key map's short-help bindings.
func shortKeys(km help.KeyMap) []string {
	keyList := make([]string, 0, len(km.ShortHelp()))
	for _, b := range km.ShortHelp() {
		keyList = append(keyList, b.Help().Key)
	}

	return keyList
}

// fullDescs returns the descriptions of every binding across a key map's
// full-help columns, flattened.
func fullDescs(km help.KeyMap) []string {
	var descs []string

	for _, col := range km.FullHelp() {
		for _, b := range col {
			descs = append(descs, b.Help().Desc)
		}
	}

	return descs
}

// TestHelp_BrowserShowsEditKey is the #681 regression: the browser page's most
// important mutation key — `e` edit — must reach the help bar, both in the
// composed key map and in the rendered help line. Before the fix the bar showed
// only the global tab/help/quit and `e` appeared nowhere on screen.
func TestHelp_BrowserShowsEditKey(t *testing.T) {
	t.Parallel()

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), nil),
	})
	m.width, m.height = browserTermWidth, browserTermHeight

	km := m.helpKeyMap()
	assert.Contains(t, shortDescs(km), "edit", "the browser help bar advertises the edit action")
	assert.Contains(t, shortKeys(km), "e", "the browser help bar advertises the `e` key")
	// The rendered help line is present (styled); the golden pins its exact form.
	assert.NotEmpty(t, m.helpView(), "the help line renders")
}

// TestHelp_DiffersByPage proves the help bar is per-page: the browser, the
// staging page, and the empty-shell placeholder each yield a different composed
// help. A placeholder falls back to the shell-only keys; the real pages layer
// their own bindings ahead of them.
func TestHelp_DiffersByPage(t *testing.T) {
	t.Parallel()

	browserApp := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), nil),
	})
	stagingApp := stagingApp()
	placeholderApp := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}})

	browserHelp := shortDescs(browserApp.helpKeyMap())
	stagingHelp := shortDescs(stagingApp.helpKeyMap())
	placeholderHelp := shortDescs(placeholderApp.helpKeyMap())

	assert.NotEqual(t, browserHelp, stagingHelp, "browser and staging help differ")
	assert.NotEqual(t, browserHelp, placeholderHelp, "browser and placeholder help differ")

	// The placeholder is shell-only; the real pages both prepend their own keys,
	// so both are strictly longer than the shell fallback.
	assert.Greater(t, len(browserHelp), len(placeholderHelp), "browser adds page keys over the shell")
	assert.Greater(t, len(stagingHelp), len(placeholderHelp), "staging adds page keys over the shell")

	// Staging advertises unstage; the browser does not — a concrete per-page key.
	assert.Contains(t, stagingHelp, "unstage", "staging help advertises `u` unstage")
	assert.NotContains(t, browserHelp, "unstage", "the browser help does not")
}

// TestHelp_FullHelpGatesOnCapability proves the full-help columns adapt to the
// service capability: a no-history, no-tags, no-restore service (App Config)
// omits compare/tag/restore, while a versioned service with tags and restore
// (AWS secret) includes them.
func TestHelp_FullHelpGatesOnCapability(t *testing.T) {
	t.Parallel()

	appConfig := newApp(config{
		scope:     provider.AzureAppConfigScope("myapp-config"),
		sourceFor: sourceForShape("param", azureAppConfigSource(), nil),
	})
	awsSecret := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		service:   "secret",
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("secret", awsSecretSource(), nil),
	})

	appConfigFull := fullDescs(appConfig.helpKeyMap())
	awsSecretFull := fullDescs(awsSecret.helpKeyMap())

	assert.NotContains(t, appConfigFull, "compare", "App Config has no version history, so no compare")
	assert.NotContains(t, appConfigFull, "restore", "App Config has no restore")

	assert.Contains(t, awsSecretFull, "compare", "AWS secret is versioned, so compare is available")
	assert.Contains(t, awsSecretFull, "restore", "AWS secret supports restore")
	assert.Contains(t, awsSecretFull, "tag", "AWS secret supports tags")
}

// TestBrowser_FullHelpGolden captures the browser page's adaptive full help
// (toggled open with `?`): the capability- and load-gated columns rendered as a
// grouped block, the on-screen proof that the page's keys are discoverable.
func TestBrowser_FullHelpGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		service:   "secret",
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("secret", awsSecretSource(), nil),
	})

	raw := captureBrowserKeys(t, m, "Version ID", '?')
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "edit", "the full help lists the edit key")
	require.Contains(t, screen, "restore", "the full help lists the restore key (AWS secret)")
	golden.RequireEqual(t, screen)
}
