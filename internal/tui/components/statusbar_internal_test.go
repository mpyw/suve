//declscope:namespace statusbar
//
// In-package tests of the status bar in statusbar.go (the file-stem namespace
// would be statusbarInternal, which names no unit of its own).

package components

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/styles"
)

func TestStatusBar_TargetSegments(t *testing.T) {
	t.Parallel()

	st := styles.New()
	kv := func(label, value string) string {
		return st.StatusKey.Render(label+":") + st.StatusValue.Render(value)
	}

	assert.Equal(t, []string{st.StatusKey.Render("loading…")},
		StatusBar{Styles: st, Target: provider.Scope{Provider: provider.ProviderAWS}.Target()}.targetSegments(),
		"a pending AWS target shows only the loading placeholder")
	assert.Equal(t, []string{kv("profile", "dev"), kv("account", "123"), kv("region", "us-east-1")},
		StatusBar{Styles: st, Target: provider.AWSTarget("dev", "123", "us-east-1")}.targetSegments(), "a resolved AWS target shows every part")
	azure := provider.Scope{Provider: provider.ProviderAzure, StoreName: "s", AppConfigNamespace: "dev"}
	assert.Equal(t, []string{kv("store", "s"), kv("namespace", "dev")},
		StatusBar{Styles: st, Target: azure.Target()}.targetSegments(),
		"parts without a value are skipped")
}
