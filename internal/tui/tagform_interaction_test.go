//nolint:testpackage // white-box: drives the tag dialog through the shared teatest host
package tui

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/styles"
)

// recordingMutator is a data.Mutator that records the tag call the tag dialog
// routes, so an interaction test can assert whether (and with what key) a submit
// reached the mutator. It is concurrency-safe: the tea program calls it from its
// own goroutine while the test polls from another.
type recordingMutator struct {
	cap capability.ServiceCapability

	mu           sync.Mutex
	removeCalled bool
	addCalled    bool
	tagKey       string
	staged       bool
}

func (m *recordingMutator) Capability() capability.ServiceCapability { return m.cap }

func (*recordingMutator) Create(context.Context, data.StagedKey, string, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (*recordingMutator) Update(context.Context, data.StagedKey, string, string, string, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (*recordingMutator) Delete(context.Context, data.StagedKey, bool, int, bool) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (m *recordingMutator) AddTag(_ context.Context, _ data.StagedKey, tagKey, _ string, staged bool) (data.WriteOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addCalled, m.tagKey, m.staged = true, tagKey, staged

	return data.WriteOutcome{}, nil
}

func (m *recordingMutator) RemoveTag(_ context.Context, _ data.StagedKey, tagKey string, staged bool) (data.WriteOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.removeCalled, m.tagKey, m.staged = true, tagKey, staged

	return data.WriteOutcome{}, nil
}

func (*recordingMutator) Restore(context.Context, string) (data.WriteOutcome, error) {
	return data.WriteOutcome{}, nil
}

func (m *recordingMutator) snapshot() (remove bool, add bool, tagKey string, staged bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.removeCalled, m.addCalled, m.tagKey, m.staged
}

// TestTUI_TagRemoveRoutesSelectedKey drives a non-empty Remove through the real
// event loop: toggling to Remove and completing the form routes the tag key the
// select seeded (the first existing tag), never the free-text Add key, to
// RemoveTag with the staged mode (#705).
func TestTUI_TagRemoveRoutesSelectedKey(t *testing.T) {
	t.Parallel()

	mut := &recordingMutator{cap: capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true}}
	m, cmd := dialogs.NewTagForm(dialogs.TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
		Tags: []data.Tag{{Key: "env", Value: "prod"}, {Key: "team", Value: "api"}},
	})

	tm := teatest.NewTestModel(t, newDialogHost(m, cmd), teatest.WithInitialTermSize(goldenTermWidth, goldenTermHeight))

	tm.Send(keyRightMsg()) // Add -> Remove

	for range 4 {
		tm.Send(keyEnterMsg()) // advance action -> select -> mode -> complete
	}

	// Wait for the submit to reach the mutator.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if remove, _, _, _ := mut.snapshot(); remove {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	remove, add, tagKey, staged := mut.snapshot()
	assert.True(t, remove, "a completed Remove routes to RemoveTag")
	assert.False(t, add, "a Remove never routes to AddTag")
	assert.Equal(t, "env", tagKey, "the select-seeded key (not a free-text key) is the untag target")
	assert.True(t, staged, "the untag routes staged by default")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}

// TestTUI_TagEmptyNeverOffersRemove drives the tag dialog for an entry with NO
// tags through the real event loop: the Action toggle never reaches Remove
// (right-arrow is a no-op with a single Add option), so the user is never lured
// into an unusable Remove and the "(no tags to remove)" dead-end never renders
// (#761). No untag is ever routed.
func TestTUI_TagEmptyNeverOffersRemove(t *testing.T) {
	t.Parallel()

	mut := &recordingMutator{cap: capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true}}
	m, cmd := dialogs.NewTagForm(dialogs.TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})

	host := newDialogHost(m, cmd)
	tm := teatest.NewTestModel(t, host, teatest.WithInitialTermSize(goldenTermWidth, goldenTermHeight))

	tm.Send(keyRightMsg()) // no-op: Remove is not offered, so the action stays on Add

	for range 6 {
		tm.Send(keyEnterMsg()) // an empty Add key fails required-field validation; nothing completes
	}

	// Give the loop time to process every key.
	time.Sleep(300 * time.Millisecond)

	remove, add, _, _ := mut.snapshot()
	assert.False(t, remove, "no untag is ever submitted for an entry with no tags")
	assert.False(t, add, "no tag write is submitted without a key")

	// The dead-end note is never rendered, and the action never reaches Remove.
	var buf bytes.Buffer

	_, _ = io.Copy(&buf, tm.Output())
	out := buf.String()
	assert.NotContains(t, out, "(no tags to remove)", "the dead-end note is never shown")
	assert.NotContains(t, out, "Remove tag", "the action toggle never reaches Remove")

	tm.Send(hostQuitMsg{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}
