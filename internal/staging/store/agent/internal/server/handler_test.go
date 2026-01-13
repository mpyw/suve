package server_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/server"
)

//nolint:funlen // Table-driven test with many cases
func TestHandler_HandleRequest(t *testing.T) {
	t.Parallel()

	t.Run("ping", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{Method: protocol.MethodPing})
		assert.True(t, resp.Success)
	})

	t.Run("shutdown", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{Method: protocol.MethodShutdown})
		assert.True(t, resp.Success)
	})

	t.Run("unknown method", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{Method: "Unknown"})
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "unknown method")
	})

	t.Run("stage and get entry", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		entry := staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}

		// Stage entry
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &entry,
		})
		assert.True(t, resp.Success)

		// Get entry
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		assert.True(t, resp.Success)

		var result protocol.EntryResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		require.NotNil(t, result.Entry)
		assert.Equal(t, "test-value", lo.FromPtr(result.Entry.Value))
	})

	t.Run("get entry - not found", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/nonexistent",
		})
		assert.True(t, resp.Success)

		var result protocol.EntryResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.Nil(t, result.Entry)
	})

	t.Run("stage and get tag", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		tagEntry := staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		// Stage tag
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			TagEntry:  &tagEntry,
		})
		assert.True(t, resp.Success)

		// Get tag
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		assert.True(t, resp.Success)

		var result protocol.TagResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		require.NotNil(t, result.TagEntry)
		assert.Equal(t, "prod", result.TagEntry.Add["env"])
	})

	t.Run("get tag - not found", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/nonexistent",
		})
		assert.True(t, resp.Success)

		var result protocol.TagResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.Nil(t, result.TagEntry)
	})

	t.Run("list entries", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage entries
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config1",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value1"), StagedAt: time.Now()},
		})
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config2",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value2"), StagedAt: time.Now()},
		})

		// List all entries
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodListEntries,
			AccountID: "123456789012",
			Region:    "us-east-1",
		})
		assert.True(t, resp.Success)

		var result protocol.ListEntriesResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.Len(t, result.Entries[staging.ServiceParam], 2)
	})

	t.Run("list entries - filter by service", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage entries in different services
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("param-value"), StagedAt: time.Now()},
		})
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceSecret,
			Name:      "my-secret",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("secret-value"), StagedAt: time.Now()},
		})

		// List param entries only
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodListEntries,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
		})
		assert.True(t, resp.Success)

		var result protocol.ListEntriesResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.Len(t, result.Entries, 1)
		assert.Contains(t, result.Entries, staging.ServiceParam)
	})

	t.Run("list tags", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage tags
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			TagEntry:  &staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: time.Now()},
		})

		// List tags
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodListTags,
			AccountID: "123456789012",
			Region:    "us-east-1",
		})
		assert.True(t, resp.Success)

		var result protocol.ListTagsResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.Len(t, result.Tags[staging.ServiceParam], 1)
	})

	t.Run("list tags - filter by service", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage tags in different services
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			TagEntry:  &staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: time.Now()},
		})
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceSecret,
			Name:      "my-secret",
			TagEntry:  &staging.TagEntry{Add: map[string]string{"team": "backend"}, StagedAt: time.Now()},
		})

		// List secret tags only
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodListTags,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceSecret,
		})
		assert.True(t, resp.Success)

		var result protocol.ListTagsResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.Len(t, result.Tags, 1)
		assert.Contains(t, result.Tags, staging.ServiceSecret)
	})

	t.Run("unstage entry", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage entry
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})

		// Unstage entry
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodUnstageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		assert.True(t, resp.Success)

		// Verify entry is gone
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		var result protocol.EntryResponse
		_ = json.Unmarshal(resp.Data, &result)
		assert.Nil(t, result.Entry)
	})

	t.Run("unstage entry - not staged", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodUnstageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/nonexistent",
		})
		assert.False(t, resp.Success)
		assert.Equal(t, staging.ErrNotStaged.Error(), resp.Error)
	})

	t.Run("unstage tag", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage tag
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			TagEntry:  &staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: time.Now()},
		})

		// Unstage tag
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodUnstageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		assert.True(t, resp.Success)

		// Verify tag is gone
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		var result protocol.TagResponse
		_ = json.Unmarshal(resp.Data, &result)
		assert.Nil(t, result.TagEntry)
	})

	t.Run("unstage tag - not staged", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodUnstageTag,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/nonexistent",
		})
		assert.False(t, resp.Success)
		assert.Equal(t, staging.ErrNotStaged.Error(), resp.Error)
	})

	t.Run("unstage all - all services", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage entries in both services
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceSecret,
			Name:      "my-secret",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})

		// Unstage all
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodUnstageAll,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   "",
		})
		assert.True(t, resp.Success)

		// Verify all entries are gone
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodListEntries,
			AccountID: "123456789012",
			Region:    "us-east-1",
		})
		var result protocol.ListEntriesResponse
		_ = json.Unmarshal(resp.Data, &result)
		assert.Empty(t, result.Entries[staging.ServiceParam])
		assert.Empty(t, result.Entries[staging.ServiceSecret])
	})

	t.Run("unstage all - specific service", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage entries in both services
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceSecret,
			Name:      "my-secret",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})

		// Unstage only param service
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodUnstageAll,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
		})
		assert.True(t, resp.Success)

		// Verify param is empty but secret still exists
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodListEntries,
			AccountID: "123456789012",
			Region:    "us-east-1",
		})
		var result protocol.ListEntriesResponse
		_ = json.Unmarshal(resp.Data, &result)
		assert.Empty(t, result.Entries[staging.ServiceParam])
		assert.Len(t, result.Entries[staging.ServiceSecret], 1)
	})

	t.Run("get state and set state", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}

		// Set state
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodSetState,
			AccountID: "123456789012",
			Region:    "us-east-1",
			State:     state,
		})
		assert.True(t, resp.Success)

		// Get state
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetState,
			AccountID: "123456789012",
			Region:    "us-east-1",
		})
		assert.True(t, resp.Success)

		var result protocol.StateResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		require.NotNil(t, result.State)
		assert.Equal(t, "test-value", lo.FromPtr(result.State.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("set state - nil state error", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodSetState,
			AccountID: "123456789012",
			Region:    "us-east-1",
			State:     nil,
		})
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "state is required")
	})

	t.Run("load - same as get state", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage an entry
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})

		// Load
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodLoad,
			AccountID: "123456789012",
			Region:    "us-east-1",
		})
		assert.True(t, resp.Success)

		var result protocol.StateResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		require.NotNil(t, result.State)
		assert.Contains(t, result.State.Entries[staging.ServiceParam], "/app/config")
	})

	t.Run("is empty - initially empty", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		assert.True(t, h.IsEmpty())

		resp := h.HandleRequest(&protocol.Request{Method: protocol.MethodIsEmpty})
		assert.True(t, resp.Success)

		var result protocol.IsEmptyResponse
		err := json.Unmarshal(resp.Data, &result)
		require.NoError(t, err)
		assert.True(t, result.Empty)
	})

	t.Run("is empty - not empty after staging", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage an entry
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "123456789012",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("value"), StagedAt: time.Now()},
		})

		assert.False(t, h.IsEmpty())

		resp := h.HandleRequest(&protocol.Request{Method: protocol.MethodIsEmpty})
		var result protocol.IsEmptyResponse
		_ = json.Unmarshal(resp.Data, &result)
		assert.False(t, result.Empty)
	})

	t.Run("multi-account/region isolation", func(t *testing.T) {
		t.Parallel()
		h := server.NewHandler()
		defer h.Destroy()

		// Stage entry in account 1
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "111111111111",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("account1-value"), StagedAt: time.Now()},
		})

		// Stage entry in account 2
		h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: "222222222222",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
			Entry:     &staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("account2-value"), StagedAt: time.Now()},
		})

		// Get entry from account 1
		resp := h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetEntry,
			AccountID: "111111111111",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		var result1 protocol.EntryResponse
		_ = json.Unmarshal(resp.Data, &result1)
		assert.Equal(t, "account1-value", lo.FromPtr(result1.Entry.Value))

		// Get entry from account 2
		resp = h.HandleRequest(&protocol.Request{
			Method:    protocol.MethodGetEntry,
			AccountID: "222222222222",
			Region:    "us-east-1",
			Service:   staging.ServiceParam,
			Name:      "/app/config",
		})
		var result2 protocol.EntryResponse
		_ = json.Unmarshal(resp.Data, &result2)
		assert.Equal(t, "account2-value", lo.FromPtr(result2.Entry.Value))
	})
}
