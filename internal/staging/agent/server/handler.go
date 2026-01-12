package server

import (
	"encoding/json"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/protocol"
)

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
func (d *Daemon) handlePing() *protocol.Response {
	return successResponse()
}

// handleGetEntry handles the GetEntry method.
func (d *Daemon) handleGetEntry(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
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
func (d *Daemon) handleGetTag(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
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
func (d *Daemon) handleListEntries(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
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
func (d *Daemon) handleListTags(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
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
func (d *Daemon) handleLoad(req *protocol.Request) *protocol.Response {
	return d.handleGetState(req)
}

// handleStageEntry handles the StageEntry method.
func (d *Daemon) handleStageEntry(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if state.Entries[req.Service] == nil {
		state.Entries[req.Service] = make(map[string]staging.Entry)
	}
	state.Entries[req.Service][req.Name] = *req.Entry

	if err := d.state.set(req.AccountID, req.Region, state); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleStageTag handles the StageTag method.
func (d *Daemon) handleStageTag(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if state.Tags[req.Service] == nil {
		state.Tags[req.Service] = make(map[string]staging.TagEntry)
	}
	state.Tags[req.Service][req.Name] = *req.TagEntry

	if err := d.state.set(req.AccountID, req.Region, state); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleUnstageEntry handles the UnstageEntry method.
func (d *Daemon) handleUnstageEntry(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if entries, ok := state.Entries[req.Service]; ok {
		if _, ok := entries[req.Name]; ok {
			delete(entries, req.Name)
			if err := d.state.set(req.AccountID, req.Region, state); err != nil {
				return errorResponse(err)
			}
			return successResponse()
		}
	}
	return errorResponse(staging.ErrNotStaged)
}

// handleUnstageTag handles the UnstageTag method.
func (d *Daemon) handleUnstageTag(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}

	if tags, ok := state.Tags[req.Service]; ok {
		if _, ok := tags[req.Name]; ok {
			delete(tags, req.Name)
			if err := d.state.set(req.AccountID, req.Region, state); err != nil {
				return errorResponse(err)
			}
			return successResponse()
		}
	}
	return errorResponse(staging.ErrNotStaged)
}

// handleUnstageAll handles the UnstageAll method.
func (d *Daemon) handleUnstageAll(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
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

	if err := d.state.set(req.AccountID, req.Region, state); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleGetState handles the GetState method (for persist).
func (d *Daemon) handleGetState(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return errorResponse(err)
	}
	return marshalResponse(protocol.StateResponse{State: state})
}

// handleSetState handles the SetState method (for drain).
func (d *Daemon) handleSetState(req *protocol.Request) *protocol.Response {
	if req.State == nil {
		return errorMessageResponse("state is required")
	}

	if err := d.state.set(req.AccountID, req.Region, req.State); err != nil {
		return errorResponse(err)
	}
	return successResponse()
}

// handleIsEmpty handles the IsEmpty method.
func (d *Daemon) handleIsEmpty() *protocol.Response {
	return marshalResponse(protocol.IsEmptyResponse{Empty: d.state.isEmpty()})
}
