//nolint:testpackage // white-box: drives the entry dialog through the shared teatest host
package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/styles"
)

// recordingEntryMutator records the create the entry dialog routes, so an
// interaction test can assert the value AND the field below it (Description) both
// reach the mutator — i.e. Tab off the Value textarea reaches Description instead of
// skipping it. Concurrency-safe: the tea program calls it from its own goroutine
// while the test polls from another.
type recordingEntryMutator struct {
	cap capability.ServiceCapability

	mu          sync.Mutex
	created     bool
	value       string
	description string
	staged      bool
}

func (m *recordingEntryMutator) Capability() capability.ServiceCapability { return m.cap }

func (m *recordingEntryMutator) Create(_ context.Context, _ data.StagedKey, value, _, description string, staged bool) (data.WriteOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.created, m.value, m.description, m.staged = true, value, description, staged

	return data.WriteOutcome{}, nil
}

func (*recordingEntryMutator) Update(context.Context, data.StagedKey, string, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (*recordingEntryMutator) Delete(context.Context, data.StagedKey, bool, int, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (*recordingEntryMutator) AddTag(context.Context, data.StagedKey, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (*recordingEntryMutator) RemoveTag(context.Context, data.StagedKey, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (*recordingEntryMutator) Restore(context.Context, string) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (m *recordingEntryMutator) snapshot() (created bool, value, description string, staged bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.created, m.value, m.description, m.staged
}

// TestTUI_EntryTabToOKSubmits drives a create through the real event loop with the
// Tab/OK model: Enter in the multi-line Value/Description inserts newlines, Tab walks
// Value → Description → the "[ OK ]" button, Enter on OK completes the form, and
// Enter in the Stage/Apply popup commits with the default (staged) mode. It proves a
// multi-line Description typed after the Value is NOT skipped.
func TestTUI_EntryTabToOKSubmits(t *testing.T) {
	t.Parallel()

	mut := &recordingEntryMutator{cap: capability.ServiceCapability{
		Service: "secret", HasStaging: true, HasDescription: true,
	}}
	m, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "secret", Styles: styles.New(),
	})

	tm := teatest.NewTestModel(t, newDialogHost(m, cmd), teatest.WithInitialTermSize(goldenTermWidth, goldenTermHeight))

	send := func(k tea.KeyPressMsg) {
		tm.Send(k)
		time.Sleep(25 * time.Millisecond)
	}
	typeStr := func(s string) {
		for _, r := range s {
			send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	tab := func() { send(tea.KeyPressMsg{Code: tea.KeyTab}) }
	enter := func() { send(tea.KeyPressMsg{Code: tea.KeyEnter}) }

	typeStr("myname")
	enter()            // Name (single-line) -> Value
	typeStr("myvalue") // Enter here would be a newline, so Tab advances instead
	tab()              // Value -> Description
	typeStr("mydesc")
	tab()   // Description -> [ OK ]
	enter() // OK -> complete -> Stage/Apply popup (async)

	// Enter on [ OK ] completes the huh form, but the form reaches StateCompleted
	// through a nextField command roundtrip, so beginSubmit only opens the
	// Stage/Apply popup a loop iteration later. Gate the commit Enter below on the
	// popup being RENDERED — under CI load a blind Enter would otherwise arrive
	// before the popup is up, be forwarded back into the completed form, and never
	// reach the mutator (created stays false, #796 class). waitFor matches the
	// vt-rendered screen, not the raw stream.
	waitFor(t, tm, "Apply immediately")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter}) // popup: commit (staged default)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if created, _, _, _ := mut.snapshot(); created {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	created, value, description, staged := mut.snapshot()
	assert.True(t, created, "the create reaches the mutator")
	assert.Equal(t, "myvalue", value, "the value is written")
	assert.Equal(t, "mydesc", description, "the multi-line Description is NOT skipped")
	assert.True(t, staged, "the default popup choice stages the write")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}
