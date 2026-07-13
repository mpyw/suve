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
// reach the mutator — i.e. ctrl+s ("done") on the Value textarea advances to
// Description instead of skipping it. Concurrency-safe: the tea program calls it
// from its own goroutine while the test polls from another.
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

// TestTUI_EntryCtrlSAdvancesToDescription drives a create through the real event
// loop: on the Value textarea ctrl+s ("done") confirms the value and advances to
// Description (rather than submitting the form and skipping it), so a Description
// typed after ctrl+s reaches the write. The final Enter on Description completes the
// form, and Enter in the Stage/Apply popup commits with the default (staged) mode.
func TestTUI_EntryCtrlSAdvancesToDescription(t *testing.T) {
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

	typeStr("myname")
	send(tea.KeyPressMsg{Code: tea.KeyEnter}) // Name -> Value
	typeStr("myvalue")
	send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) // Value: confirm + advance -> Description
	typeStr("mydesc")
	send(tea.KeyPressMsg{Code: tea.KeyEnter}) // Description (last) -> complete -> popup
	send(tea.KeyPressMsg{Code: tea.KeyEnter}) // popup: commit (staged default)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if created, _, _, _ := mut.snapshot(); created {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	created, value, description, staged := mut.snapshot()
	assert.True(t, created, "the create reaches the mutator")
	assert.Equal(t, "myvalue", value, "the value is written")
	assert.Equal(t, "mydesc", description, "the Description typed after ctrl+s is NOT skipped")
	assert.True(t, staged, "the default popup choice stages the write")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}
