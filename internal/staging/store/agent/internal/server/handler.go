// Package server provides request handling for the staging agent.
package server

import (
	"encoding/json"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

// Handler handles staging agent requests.
type Handler struct {
	state *secureState
}

// NewHandler creates a new request handler.
func NewHandler() *Handler {
	return &Handler{
		state: newSecureState(),
	}
}

// IsEmpty returns true if all states are empty.
func (h *Handler) IsEmpty() bool {
	return h.state.isEmpty()
}

// Destroy securely destroys all state data.
func (h *Handler) Destroy() {
	h.state.destroy()
}

// HandleRequest processes a request and returns a response.
func (h *Handler) HandleRequest(req *protocol.Request) *protocol.Response {
	switch req.Method {
	case protocol.MethodPing:
		return h.handlePing()
	case protocol.MethodShutdown:
		// Shutdown is handled by the daemon runner via response callback
		return successResponse()
	case protocol.MethodGetEntry:
		return h.handleGetEntry(req)
	case protocol.MethodGetTag:
		return h.handleGetTag(req)
	case protocol.MethodListEntries:
		return h.handleListEntries(req)
	case protocol.MethodListTags:
		return h.handleListTags(req)
	case protocol.MethodLoad:
		return h.handleLoad(req)
	case protocol.MethodStageEntry:
		return h.handleStageEntry(req)
	case protocol.MethodStageTag:
		return h.handleStageTag(req)
	case protocol.MethodUnstageEntry:
		return h.handleUnstageEntry(req)
	case protocol.MethodUnstageTag:
		return h.handleUnstageTag(req)
	case protocol.MethodUnstageAll:
		return h.handleUnstageAll(req)
	case protocol.MethodGetState:
		return h.handleGetState(req)
	case protocol.MethodSetState:
		return h.handleSetState(req)
	case protocol.MethodIsEmpty:
		return h.handleIsEmpty()
	default:
		return errorMessageResponse("unknown method: " + req.Method)
	}
}

// successResponse returns a successful response without data.
func successResponse() *protocol.Response {
	return &protocol.Response{Success: true}
}

// errorResponse returns an error response.
func errorResponse(err error) *protocol.Response {
	return &protocol.Response{Success: false, Error: err.Error()}
}

// errorMessageResponse returns an error response with a message.
func errorMessageResponse(msg string) *protocol.Response {
	return &protocol.Response{Success: false, Error: msg}
}

// marshalResponse is a helper to marshal data and return a response.
func marshalResponse(v any) *protocol.Response {
	data, err := json.Marshal(v)
	if err != nil {
		return errorResponse(err)
	}
	return &protocol.Response{Success: true, Data: data}
}

// handlePing handles the Ping method.
func (h *Handler) handlePing() *protocol.Response {
	return successResponse()
}

// handleGetEntry handles the GetEntry method.
func (h *Handler) handleGetEntry(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	var entry *staging.Entry
	if entries, ok := state.Entries[req.Service]; ok {
		if e, ok := entries[req.Name]; ok {
			entry = &e
		}
	}
	return marshalResponse(protocol.EntryResponse{Entry: entry})
}

// handleGetTag handles the GetTag method.
func (h *Handler) handleGetTag(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	var tagEntry *staging.TagEntry
	if tags, ok := state.Tags[req.Service]; ok {
		if t, ok := tags[req.Name]; ok {
			tagEntry = &t
		}
	}
	return marshalResponse(protocol.TagResponse{TagEntry: tagEntry})
}

// handleListEntries handles the ListEntries method.
func (h *Handler) handleListEntries(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	entries := state.Entries
	if req.Service != "" {
		// Filter by service
		entries = make(map[staging.Service]map[string]staging.Entry)
		if svcEntries, ok := state.Entries[req.Service]; ok {
			entries[req.Service] = svcEntries
		}
	}
	return marshalResponse(protocol.ListEntriesResponse{Entries: entries})
}

// handleListTags handles the ListTags method.
func (h *Handler) handleListTags(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	tags := state.Tags
	if req.Service != "" {
		// Filter by service
		tags = make(map[staging.Service]map[string]staging.TagEntry)
		if svcTags, ok := state.Tags[req.Service]; ok {
			tags[req.Service] = svcTags
		}
	}
	return marshalResponse(protocol.ListTagsResponse{Tags: tags})
}

// handleLoad handles the Load method.
func (h *Handler) handleLoad(req *protocol.Request) *protocol.Response {
	return h.handleGetState(req)
}

// handleStageEntry handles the StageEntry method.
func (h *Handler) handleStageEntry(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if state.Entries[req.Service] == nil {
		state.Entries[req.Service] = make(map[string]staging.Entry)
	}
	state.Entries[req.Service][req.Name] = *req.Entry

	if err := h.state.set(req.AccountID, req.Region, state); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleStageTag handles the StageTag method.
func (h *Handler) handleStageTag(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if state.Tags[req.Service] == nil {
		state.Tags[req.Service] = make(map[string]staging.TagEntry)
	}
	state.Tags[req.Service][req.Name] = *req.TagEntry

	if err := h.state.set(req.AccountID, req.Region, state); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleUnstageEntry handles the UnstageEntry method.
func (h *Handler) handleUnstageEntry(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if entries, ok := state.Entries[req.Service]; ok {
		if _, ok := entries[req.Name]; ok {
			delete(entries, req.Name)
			if err := h.state.set(req.AccountID, req.Region, state); err != nil {
				return errorResponse(err)
			}
			return successResponse()
		}
	}
	return errorResponse(staging.ErrNotStaged)
}

// handleUnstageTag handles the UnstageTag method.
func (h *Handler) handleUnstageTag(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if tags, ok := state.Tags[req.Service]; ok {
		if _, ok := tags[req.Name]; ok {
			delete(tags, req.Name)
			if err := h.state.set(req.AccountID, req.Region, state); err != nil {
				return errorResponse(err)
			}
			return successResponse()
		}
	}
	return errorResponse(staging.ErrNotStaged)
}

// handleUnstageAll handles the UnstageAll method.
func (h *Handler) handleUnstageAll(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	// Clear all entries and tags for all services if req.Service is empty
	if req.Service == "" {
		state.Entries = map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		}
		state.Tags = map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		}
	} else {
		// Clear only the specified service
		state.Entries[req.Service] = make(map[string]staging.Entry)
		state.Tags[req.Service] = make(map[string]staging.TagEntry)
	}

	if err := h.state.set(req.AccountID, req.Region, state); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleGetState handles the GetState method (for persist).
func (h *Handler) handleGetState(req *protocol.Request) *protocol.Response {
	state, err := h.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}
	return marshalResponse(protocol.StateResponse{State: state})
}

// handleSetState handles the SetState method (for drain).
func (h *Handler) handleSetState(req *protocol.Request) *protocol.Response {
	if req.State == nil {
		return errorMessageResponse("state is required")
	}

	if err := h.state.set(req.AccountID, req.Region, req.State); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleIsEmpty handles the IsEmpty method.
func (h *Handler) handleIsEmpty() *protocol.Response {
	return marshalResponse(protocol.IsEmptyResponse{Empty: h.state.isEmpty()})
}
