package client

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
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
	entry, err := s.client.GetEntry(ctx, s.accountID, s.region, service, name)
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
	tagEntry, err := s.client.GetTag(ctx, s.accountID, s.region, service, name)
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
	return s.client.ListEntries(ctx, s.accountID, s.region, service)
}

// ListTags returns all staged tag changes for a service.
func (s *Store) ListTags(ctx context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	return s.client.ListTags(ctx, s.accountID, s.region, service)
}

// Load loads the current staging state.
func (s *Store) Load(ctx context.Context) (*staging.State, error) {
	return s.client.Load(ctx, s.accountID, s.region)
}

// StageEntry adds or updates a staged entry.
func (s *Store) StageEntry(ctx context.Context, service staging.Service, name string, entry staging.Entry) error {
	return s.client.StageEntry(ctx, s.accountID, s.region, service, name, entry)
}

// StageTag adds or updates staged tag changes.
func (s *Store) StageTag(ctx context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error {
	return s.client.StageTag(ctx, s.accountID, s.region, service, name, tagEntry)
}

// UnstageEntry removes a staged entry.
func (s *Store) UnstageEntry(ctx context.Context, service staging.Service, name string) error {
	return s.client.UnstageEntry(ctx, s.accountID, s.region, service, name)
}

// UnstageTag removes staged tag changes.
func (s *Store) UnstageTag(ctx context.Context, service staging.Service, name string) error {
	return s.client.UnstageTag(ctx, s.accountID, s.region, service, name)
}

// UnstageAll removes all staged changes for a service.
func (s *Store) UnstageAll(ctx context.Context, service staging.Service) error {
	return s.client.UnstageAll(ctx, s.accountID, s.region, service)
}

// Drain retrieves the state from the daemon, optionally clearing memory.
// This implements StateDrainer for agent-based storage.
// If keep is false, the daemon memory is cleared after reading.
func (s *Store) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	state, err := s.client.GetState(ctx, s.accountID, s.region)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = staging.NewEmptyState()
	}

	// Clear memory if keep is false
	if !keep {
		if err := s.client.UnstageAll(ctx, s.accountID, s.region, ""); err != nil {
			return nil, err
		}
	}

	return state, nil
}

// Compile-time check that Store implements StoreReadWriteOperator.
var _ staging.StoreReadWriteOperator = (*Store)(nil)

// Compile-time check that Store implements StateDrainer.
var _ staging.StateDrainer = (*Store)(nil)
