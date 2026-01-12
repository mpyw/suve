package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/protocol"
)

// Store implements staging.StoreReadWriteOperator using the daemon.
type Store struct {
	client    *Client
	accountID string
	region    string
}

// NewStore creates a new Store.
func NewStore(accountID, region string, opts ...ClientOption) *Store {
	return &Store{
		client:    NewClient(opts...),
		accountID: accountID,
		region:    region,
	}
}

// GetEntry retrieves a staged entry.
func (s *Store) GetEntry(ctx context.Context, service staging.Service, name string) (*staging.Entry, error) {
	entry, err := doRequestWithResultEnsuringDaemon(s, ctx, &protocol.Request{
		Method:    protocol.MethodGetEntry,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
		Name:      name,
	}, func(r *protocol.EntryResponse) *staging.Entry { return r.Entry })
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, staging.ErrNotStaged
	}
	return entry, nil
}

// GetTag retrieves staged tag changes.
func (s *Store) GetTag(ctx context.Context, service staging.Service, name string) (*staging.TagEntry, error) {
	tagEntry, err := doRequestWithResultEnsuringDaemon(s, ctx, &protocol.Request{
		Method:    protocol.MethodGetTag,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
		Name:      name,
	}, func(r *protocol.TagResponse) *staging.TagEntry { return r.TagEntry })
	if err != nil {
		return nil, err
	}
	if tagEntry == nil {
		return nil, staging.ErrNotStaged
	}
	return tagEntry, nil
}

// ListEntries returns all staged entries for a service.
func (s *Store) ListEntries(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
	return doRequestWithResultEnsuringDaemon(s, ctx, &protocol.Request{
		Method:    protocol.MethodListEntries,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
	}, func(r *protocol.ListEntriesResponse) map[staging.Service]map[string]staging.Entry { return r.Entries })
}

// ListTags returns all staged tag changes for a service.
func (s *Store) ListTags(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	return doRequestWithResultEnsuringDaemon(s, ctx, &protocol.Request{
		Method:    protocol.MethodListTags,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
	}, func(r *protocol.ListTagsResponse) map[staging.Service]map[string]staging.TagEntry { return r.Tags })
}

// Load loads the current staging state.
func (s *Store) Load(ctx context.Context) (*staging.State, error) {
	return doRequestWithResultEnsuringDaemon(s, ctx, &protocol.Request{
		Method:    protocol.MethodLoad,
		AccountID: s.accountID,
		Region:    s.region,
	}, func(r *protocol.StateResponse) *staging.State { return r.State })
}

// StageEntry adds or updates a staged entry.
func (s *Store) StageEntry(ctx context.Context, service staging.Service, name string, entry staging.Entry) error {
	return s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
		Method:    protocol.MethodStageEntry,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
		Name:      name,
		Entry:     &entry,
	})
}

// StageTag adds or updates staged tag changes.
func (s *Store) StageTag(ctx context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error {
	return s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
		Method:    protocol.MethodStageTag,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
		Name:      name,
		TagEntry:  &tagEntry,
	})
}

// UnstageEntry removes a staged entry.
func (s *Store) UnstageEntry(ctx context.Context, service staging.Service, name string) error {
	return s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
		Method:    protocol.MethodUnstageEntry,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
		Name:      name,
	})
}

// UnstageTag removes staged tag changes.
func (s *Store) UnstageTag(ctx context.Context, service staging.Service, name string) error {
	return s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
		Method:    protocol.MethodUnstageTag,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
		Name:      name,
	})
}

// UnstageAll removes all staged changes for a service.
func (s *Store) UnstageAll(ctx context.Context, service staging.Service) error {
	return s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
		Method:    protocol.MethodUnstageAll,
		AccountID: s.accountID,
		Region:    s.region,
		Service:   service,
	})
}

// Drain retrieves the state from the daemon, optionally clearing memory.
// This implements StateDrainer for agent-based storage.
// If keep is false, the daemon memory is cleared after reading.
func (s *Store) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	state, err := doRequestWithResult(s, ctx, &protocol.Request{
		Method:    protocol.MethodGetState,
		AccountID: s.accountID,
		Region:    s.region,
	}, func(r *protocol.StateResponse) *staging.State { return r.State })
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = staging.NewEmptyState()
	}

	// Clear memory if keep is false
	if !keep {
		if err := s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
			Method:    protocol.MethodUnstageAll,
			AccountID: s.accountID,
			Region:    s.region,
			Service:   "",
		}); err != nil {
			return nil, err
		}
	}

	return state, nil
}

// SetState sets the full state for drain operations.
func (s *Store) SetState(ctx context.Context, state *staging.State) error {
	return s.doSimpleRequestEnsuringDaemon(ctx, &protocol.Request{
		Method:    protocol.MethodSetState,
		AccountID: s.accountID,
		Region:    s.region,
		State:     state,
	})
}

// doRequestWithResult sends a request to the daemon and unmarshals the response.
func doRequestWithResult[Resp any, Result any](
	s *Store,
	ctx context.Context,
	req *protocol.Request,
	extract func(*Resp) Result,
) (Result, error) {
	var zero Result

	resp, err := s.client.sendRequest(ctx, req)
	if err != nil {
		return zero, err
	}
	if err := resp.Err(); err != nil {
		return zero, err
	}

	var result Resp
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return extract(&result), nil
}

// doRequestWithResultEnsuringDaemon ensures the daemon is running, then sends a request and unmarshals the response.
func doRequestWithResultEnsuringDaemon[Resp any, Result any](
	s *Store,
	ctx context.Context,
	req *protocol.Request,
	extract func(*Resp) Result,
) (Result, error) {
	var zero Result
	if err := s.client.ensureDaemon(ctx); err != nil {
		return zero, err
	}
	return doRequestWithResult(s, ctx, req, extract)
}

// doSimpleRequestEnsuringDaemon ensures the daemon is running, then sends a simple request.
func (s *Store) doSimpleRequestEnsuringDaemon(ctx context.Context, req *protocol.Request) error {
	if err := s.client.ensureDaemon(ctx); err != nil {
		return err
	}
	return s.client.doSimpleRequest(ctx, req)
}

// Compile-time check that Store implements StoreReadWriteOperator.
var _ staging.StoreReadWriteOperator = (*Store)(nil)

// Compile-time check that Store implements StateDrainer.
var _ staging.StateDrainer = (*Store)(nil)
