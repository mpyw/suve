package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

// testDaemon creates a daemon for testing.
func testDaemon() *Daemon {
	return &Daemon{
		state: newSecureState(),
	}
}

func TestHandlePing(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	resp := d.handlePing()
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)
}

func TestHandleStageEntry(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	entry := staging.Entry{
		Operation: staging.OperationCreate,
		Value:     strPtr("test-value"),
		StagedAt:  time.Now(),
	}

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
		Entry:     &entry,
	}

	resp := d.handleStageEntry(req)
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)

	// Verify state
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Len(t, state.Entries[staging.ServiceParam], 1)
	assert.Equal(t, "test-value", *state.Entries[staging.ServiceParam]["/my/param"].Value)
}

func TestHandleStageTag(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	tagEntry := staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceSecret,
		Name:      "my-secret",
		TagEntry:  &tagEntry,
	}

	resp := d.handleStageTag(req)
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)

	// Verify state
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Len(t, state.Tags[staging.ServiceSecret], 1)
	assert.Equal(t, "prod", state.Tags[staging.ServiceSecret]["my-secret"].Add["env"])
}

func TestHandleGetEntry(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage an entry first
	entry := staging.Entry{
		Operation: staging.OperationCreate,
		Value:     strPtr("test-value"),
		StagedAt:  time.Now(),
	}
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
		Entry:     &entry,
	})

	// Get the entry
	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
	}

	resp := d.handleGetEntry(req)
	assert.True(t, resp.Success)

	var result EntryResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Entry)
	assert.Equal(t, "test-value", *result.Entry.Value)
}

func TestHandleGetEntry_NotFound(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/nonexistent",
	}

	resp := d.handleGetEntry(req)
	assert.True(t, resp.Success) // Success is true but entry is nil

	var result EntryResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	assert.Nil(t, result.Entry)
}

func TestHandleGetTag(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage a tag first
	tagEntry := staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}
	_ = d.handleStageTag(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
		TagEntry:  &tagEntry,
	})

	// Get the tag
	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
	}

	resp := d.handleGetTag(req)
	assert.True(t, resp.Success)

	var result TagResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result.TagEntry)
	assert.Equal(t, "prod", result.TagEntry.Add["env"])
}

func TestHandleListEntries(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage entries
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceSecret,
		Name:      "secret1",
		Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: strPtr("s1"), StagedAt: time.Now()},
	})

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
	}

	resp := d.handleListEntries(req)
	assert.True(t, resp.Success)

	var result ListEntriesResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	assert.Len(t, result.Entries[staging.ServiceParam], 1)
	assert.Len(t, result.Entries[staging.ServiceSecret], 1)
}

func TestHandleListTags(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage tags
	_ = d.handleStageTag(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		TagEntry:  &staging.TagEntry{Add: map[string]string{"k1": "v1"}, StagedAt: time.Now()},
	})

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
	}

	resp := d.handleListTags(req)
	assert.True(t, resp.Success)

	var result ListTagsResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	assert.Len(t, result.Tags[staging.ServiceParam], 1)
}

func TestHandleLoad(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage some data
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
	}

	resp := d.handleLoad(req)
	assert.True(t, resp.Success)

	var result StateResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result.State)
	assert.Len(t, result.State.Entries[staging.ServiceParam], 1)
}

func TestHandleUnstageEntry(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage an entry
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})

	// Unstage it
	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
	}

	resp := d.handleUnstageEntry(req)
	assert.True(t, resp.Success)

	// Verify it's gone
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Empty(t, state.Entries[staging.ServiceParam])
}

func TestHandleUnstageEntry_NotStaged(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/nonexistent",
	}

	resp := d.handleUnstageEntry(req)
	assert.False(t, resp.Success)
	assert.Equal(t, staging.ErrNotStaged.Error(), resp.Error)
}

func TestHandleUnstageTag(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage a tag
	_ = d.handleStageTag(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
		TagEntry:  &staging.TagEntry{Add: map[string]string{"k1": "v1"}, StagedAt: time.Now()},
	})

	// Unstage it
	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/my/param",
	}

	resp := d.handleUnstageTag(req)
	assert.True(t, resp.Success)

	// Verify it's gone
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Empty(t, state.Tags[staging.ServiceParam])
}

func TestHandleUnstageAll(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage entries for both services
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceSecret,
		Name:      "secret1",
		Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: strPtr("s1"), StagedAt: time.Now()},
	})

	// Unstage all for param service only
	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
	}

	resp := d.handleUnstageAll(req)
	assert.True(t, resp.Success)

	// Verify param is cleared but secret remains
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Empty(t, state.Entries[staging.ServiceParam])
	assert.Len(t, state.Entries[staging.ServiceSecret], 1)
}

func TestHandleUnstageAll_AllServices(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage entries for both services
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceSecret,
		Name:      "secret1",
		Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: strPtr("s1"), StagedAt: time.Now()},
	})

	// Unstage all for all services
	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   "", // Empty means all services
	}

	resp := d.handleUnstageAll(req)
	assert.True(t, resp.Success)

	// Verify all are cleared
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Empty(t, state.Entries[staging.ServiceParam])
	assert.Empty(t, state.Entries[staging.ServiceSecret])
}

func TestHandleGetState(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Stage some data
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
	}

	resp := d.handleGetState(req)
	assert.True(t, resp.Success)

	var result StateResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result.State)
}

func TestHandleSetState(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	newState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/imported": {Operation: staging.OperationCreate, Value: strPtr("imported-value"), StagedAt: time.Now()},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		State:     newState,
	}

	resp := d.handleSetState(req)
	assert.True(t, resp.Success)

	// Verify state was set
	state, err := d.state.get("123456789012", "us-east-1")
	require.NoError(t, err)
	assert.Len(t, state.Entries[staging.ServiceParam], 1)
	assert.Equal(t, "imported-value", *state.Entries[staging.ServiceParam]["/imported"].Value)
}

func TestHandleSetState_NilState(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	req := &Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		State:     nil,
	}

	resp := d.handleSetState(req)
	assert.False(t, resp.Success)
	assert.Equal(t, "state is required", resp.Error)
}

func TestHandleIsEmpty(t *testing.T) {
	d := testDaemon()
	defer d.state.destroy()

	// Initially empty
	resp := d.handleIsEmpty()
	assert.True(t, resp.Success)

	var result IsEmptyResponse
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	assert.True(t, result.Empty)

	// Stage something
	_ = d.handleStageEntry(&Request{
		AccountID: "123456789012",
		Region:    "us-east-1",
		Service:   staging.ServiceParam,
		Name:      "/param1",
		Entry:     &staging.Entry{Operation: staging.OperationCreate, Value: strPtr("v1"), StagedAt: time.Now()},
	})

	// Now not empty
	resp = d.handleIsEmpty()
	assert.True(t, resp.Success)

	err = json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	assert.False(t, result.Empty)
}
