// Package protocol defines the IPC protocol between the agent client and server.
package protocol

import (
	"encoding/json"
	"errors"

	"github.com/mpyw/suve/internal/staging"
)

// Method names for the JSON-RPC protocol.
const (
	MethodPing         = "Ping"
	MethodShutdown     = "Shutdown"
	MethodGetEntry     = "GetEntry"
	MethodGetTag       = "GetTag"
	MethodListEntries  = "ListEntries"
	MethodListTags     = "ListTags"
	MethodLoad         = "Load"
	MethodStageEntry   = "StageEntry"
	MethodStageTag     = "StageTag"
	MethodUnstageEntry = "UnstageEntry"
	MethodUnstageTag   = "UnstageTag"
	MethodUnstageAll   = "UnstageAll"
	MethodGetState     = "GetState"
	MethodSetState     = "SetState"
	MethodIsEmpty      = "IsEmpty"
)

// Request represents a JSON-RPC request to the daemon.
type Request struct {
	Method    string            `json:"method"`
	AccountID string            `json:"account_id"`
	Region    string            `json:"region"`
	Service   staging.Service   `json:"service,omitempty"`
	Name      string            `json:"name,omitempty"`
	Entry     *staging.Entry    `json:"entry,omitempty"`
	TagEntry  *staging.TagEntry `json:"tag_entry,omitempty"`
	State     *staging.State    `json:"state,omitempty"`
}

// Response represents a JSON-RPC response from the daemon.
type Response struct {
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Err returns the error from the response, converting known error messages.
func (r *Response) Err() error {
	if r.Success {
		return nil
	}
	if r.Error == staging.ErrNotStaged.Error() {
		return staging.ErrNotStaged
	}
	return errors.New(r.Error)
}

// EntryResponse is the response data for GetEntry.
type EntryResponse struct {
	Entry *staging.Entry `json:"entry,omitempty"`
}

// TagResponse is the response data for GetTag.
type TagResponse struct {
	TagEntry *staging.TagEntry `json:"tag_entry,omitempty"`
}

// ListEntriesResponse is the response data for ListEntries.
type ListEntriesResponse struct {
	Entries map[staging.Service]map[string]staging.Entry `json:"entries"`
}

// ListTagsResponse is the response data for ListTags.
type ListTagsResponse struct {
	Tags map[staging.Service]map[string]staging.TagEntry `json:"tags"`
}

// StateResponse is the response data for Load and GetState.
type StateResponse struct {
	State *staging.State `json:"state"`
}

// IsEmptyResponse is the response data for IsEmpty.
type IsEmptyResponse struct {
	Empty bool `json:"empty"`
}
