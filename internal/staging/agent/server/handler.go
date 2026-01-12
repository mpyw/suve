package server

import (
	"encoding/json"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/protocol"
)

// handlePing handles the Ping method.
func (d *Daemon) handlePing() *protocol.Response {
	return &protocol.Response{Success: true}
}

// marshalResponse is a helper to marshal data and return a response.
func marshalResponse(v any) *protocol.Response {
	data, err := json.Marshal(v)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return &protocol.Response{Success: true, Data: data}
}

// handleGetEntry handles the GetEntry method.
func (d *Daemon) handleGetEntry(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
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
		return &protocol.Response{Success: false, Error: err.Error()}
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
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return marshalResponse(protocol.ListEntriesResponse{Entries: state.Entries})
}

// handleListTags handles the ListTags method.
func (d *Daemon) handleListTags(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return marshalResponse(protocol.ListTagsResponse{Tags: state.Tags})
}

// handleLoad handles the Load method.
func (d *Daemon) handleLoad(req *protocol.Request) *protocol.Response {
	return d.handleGetState(req)
}

// handleStageEntry handles the StageEntry method.
func (d *Daemon) handleStageEntry(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}

	if state.Entries[req.Service] == nil {
		state.Entries[req.Service] = make(map[string]staging.Entry)
	}
	state.Entries[req.Service][req.Name] = *req.Entry

	if err := d.state.set(req.AccountID, req.Region, state); err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return &protocol.Response{Success: true}
}

// handleStageTag handles the StageTag method.
func (d *Daemon) handleStageTag(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}

	if state.Tags[req.Service] == nil {
		state.Tags[req.Service] = make(map[string]staging.TagEntry)
	}
	state.Tags[req.Service][req.Name] = *req.TagEntry

	if err := d.state.set(req.AccountID, req.Region, state); err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return &protocol.Response{Success: true}
}

// handleUnstageEntry handles the UnstageEntry method.
func (d *Daemon) handleUnstageEntry(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}

	if entries, ok := state.Entries[req.Service]; ok {
		if _, ok := entries[req.Name]; ok {
			delete(entries, req.Name)
			if err := d.state.set(req.AccountID, req.Region, state); err != nil {
				return &protocol.Response{Success: false, Error: err.Error()}
			}
			return &protocol.Response{Success: true}
		}
	}
	return &protocol.Response{Success: false, Error: staging.ErrNotStaged.Error()}
}

// handleUnstageTag handles the UnstageTag method.
func (d *Daemon) handleUnstageTag(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}

	if tags, ok := state.Tags[req.Service]; ok {
		if _, ok := tags[req.Name]; ok {
			delete(tags, req.Name)
			if err := d.state.set(req.AccountID, req.Region, state); err != nil {
				return &protocol.Response{Success: false, Error: err.Error()}
			}
			return &protocol.Response{Success: true}
		}
	}
	return &protocol.Response{Success: false, Error: staging.ErrNotStaged.Error()}
}

// handleUnstageAll handles the UnstageAll method.
func (d *Daemon) handleUnstageAll(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
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
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return &protocol.Response{Success: true}
}

// handleGetState handles the GetState method (for persist).
func (d *Daemon) handleGetState(req *protocol.Request) *protocol.Response {
	state, err := d.state.get(req.AccountID, req.Region)
	if err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return marshalResponse(protocol.StateResponse{State: state})
}

// handleSetState handles the SetState method (for drain).
func (d *Daemon) handleSetState(req *protocol.Request) *protocol.Response {
	if req.State == nil {
		return &protocol.Response{Success: false, Error: "state is required"}
	}

	if err := d.state.set(req.AccountID, req.Region, req.State); err != nil {
		return &protocol.Response{Success: false, Error: err.Error()}
	}
	return &protocol.Response{Success: true}
}

// handleIsEmpty handles the IsEmpty method.
func (d *Daemon) handleIsEmpty() *protocol.Response {
	return marshalResponse(protocol.IsEmptyResponse{Empty: d.state.isEmpty()})
}
