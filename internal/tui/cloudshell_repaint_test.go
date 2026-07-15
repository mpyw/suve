//nolint:testpackage // white-box: exercises the unexported cloud-shell repaint loop
package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

// drainBatch runs cmd (recursing into batches) and returns every leaf message.
// It blocks on timer commands (e.g. tea.Tick), so callers accept that latency.
func drainBatch(cmd tea.Cmd) []tea.Msg {
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
			out = append(out, drainBatch(c)...)
		}

		return out
	}

	return []tea.Msg{msg}
}

// clearScreenType is the reflected type of tea's (unexported) clear-screen msg.
//
//nolint:gochecknoglobals // test-only type sentinel
var clearScreenType = reflect.TypeOf(tea.ClearScreen())

// TestCloudShellRepaintCmd_ProducesRepaintMsg pins that the repaint ticker fires
// a cloudShellRepaintMsg (so the loop keeps rescheduling itself).
func TestCloudShellRepaintCmd_ProducesRepaintMsg(t *testing.T) {
	t.Parallel()

	msg := cloudShellRepaintCmd()()
	assert.IsType(t, cloudShellRepaintMsg{}, msg)
}

// TestUpdate_CloudShellRepaint_ClearsAndReschedules pins that handling a repaint
// tick both forces a full repaint (tea.ClearScreen) and arms the next tick.
func TestUpdate_CloudShellRepaint_ClearsAndReschedules(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}})

	_, cmd := m.Update(cloudShellRepaintMsg{})
	require.NotNil(t, cmd)

	msgs := drainBatch(cmd)

	var sawClear, sawReschedule bool

	for _, msg := range msgs {
		if reflect.TypeOf(msg) == clearScreenType {
			sawClear = true
		}

		if _, ok := msg.(cloudShellRepaintMsg); ok {
			sawReschedule = true
		}
	}

	assert.True(t, sawClear, "a repaint tick must force a full ClearScreen")
	assert.True(t, sawReschedule, "a repaint tick must arm the next tick")
}
