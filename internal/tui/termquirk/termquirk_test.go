package termquirk_test

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/tui/termquirk"
)

func TestScrollNeedsFullRepaint(t *testing.T) {
	tests := []struct {
		name      string
		awsExec   string // AWS_EXECUTION_ENV
		gcpShell  string // CLOUD_SHELL
		azureHost string // AZUREPS_HOST_ENVIRONMENT
		override  string // SUVE_TUI_FULL_REPAINT
		want      bool
	}{
		{name: "native terminal (nothing set)", want: false},
		{name: "some other AWS execution env (Lambda)", awsExec: "AWS_Lambda_go1.x", want: false},
		{name: "AWS CloudShell", awsExec: "CloudShell", want: true},
		{name: "Google Cloud Shell", gcpShell: "true", want: true},
		{name: "CLOUD_SHELL set to something else -> not detected", gcpShell: "1", want: false},
		{name: "Azure Cloud Shell", azureHost: "cloud-shell/1.0", want: true},
		{name: "Azure host env unrelated -> not detected", azureHost: "AzureAutomation/1.0", want: false},
		{name: "override forces it on for other browser terminals", override: "1", want: true},
		{name: "override wins even on a native terminal", override: "true", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: t.Setenv mutates process env.
			t.Setenv("AWS_EXECUTION_ENV", tt.awsExec)
			t.Setenv("CLOUD_SHELL", tt.gcpShell)
			t.Setenv("AZUREPS_HOST_ENVIRONMENT", tt.azureHost)
			t.Setenv("SUVE_TUI_FULL_REPAINT", tt.override)

			assert.Equal(t, tt.want, termquirk.ScrollNeedsFullRepaint())
		})
	}
}

// drain runs cmd (recursing into batches) and returns every leaf message.
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

// clearScreenType is the reflected type of tea's (unexported) clear-screen
// message, obtained from a known tea.ClearScreen() invocation.
//
//nolint:gochecknoglobals // test-only type sentinel
var clearScreenType = reflect.TypeOf(tea.ClearScreen())

func containsClearScreen(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if reflect.TypeOf(m) == clearScreenType {
			return true
		}
	}

	return false
}

func containsMsg(msgs []tea.Msg, want any) bool {
	for _, m := range msgs {
		if m == want {
			return true
		}
	}

	return false
}

func TestRepaintOnScroll(t *testing.T) {
	sentinel := tea.Cmd(func() tea.Msg { return "sentinel" })

	tests := []struct {
		name       string
		scrolled   bool
		cloudShell bool
		wantClear  bool
	}{
		{name: "no scroll, native terminal -> passthrough", scrolled: false, cloudShell: false},
		{name: "no scroll, CloudShell -> passthrough (nothing to repaint)", scrolled: false, cloudShell: true},
		{name: "scroll, native terminal -> keep optimized path", scrolled: true, cloudShell: false},
		{name: "scroll, CloudShell -> force full repaint", scrolled: true, cloudShell: true, wantClear: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: t.Setenv mutates process env (read by termquirk).
			t.Setenv("CLOUD_SHELL", "")
			t.Setenv("AZUREPS_HOST_ENVIRONMENT", "")
			t.Setenv("SUVE_TUI_FULL_REPAINT", "")

			if tt.cloudShell {
				t.Setenv("AWS_EXECUTION_ENV", "CloudShell")
			} else {
				t.Setenv("AWS_EXECUTION_ENV", "")
			}

			msgs := drain(termquirk.RepaintOnScroll(tt.scrolled, sentinel))

			assert.Equal(t, tt.wantClear, containsClearScreen(msgs),
				"a full repaint must be forced only when a scroll happened on an affected terminal")
			assert.True(t, containsMsg(msgs, "sentinel"),
				"the wrapped command must always survive")
		})
	}
}

// TestRepaintOnScrollNilCmd pins that a nil cmd yields a bare ClearScreen (no
// panic) on an affected terminal, and nil on the fast path.
func TestRepaintOnScrollNilCmd(t *testing.T) {
	t.Setenv("CLOUD_SHELL", "")
	t.Setenv("AZUREPS_HOST_ENVIRONMENT", "")
	t.Setenv("SUVE_TUI_FULL_REPAINT", "")

	t.Setenv("AWS_EXECUTION_ENV", "CloudShell")
	assert.True(t, containsClearScreen(drain(termquirk.RepaintOnScroll(true, nil))))

	t.Setenv("AWS_EXECUTION_ENV", "")
	assert.Nil(t, termquirk.RepaintOnScroll(true, nil), "native terminal keeps the fast path (nil cmd)")
}
