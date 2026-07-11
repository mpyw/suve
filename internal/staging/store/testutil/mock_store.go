// Package testutil provides test utilities for staging package.
package testutil

import (
	"context"
	"maps"
	"slices"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// MockStore implements store.ReadWriteOperator for testing.
// It stores state in memory and can be configured to return errors.
type MockStore struct {
	entries map[staging.Service]map[staging.EntryKey]staging.Entry
	tags    map[staging.Service]map[staging.EntryKey]staging.TagEntry

	// Error injection for testing error paths
	GetEntryErr     error
	GetTagErr       error
	StageEntryErr   error
	StageTagErr     error
	UnstageTagErr   error
	UnstageEntryErr error
	UnstageAllErr   error
	ListEntriesErr  error
	ListTagsErr     error
	DrainErr        error
	WriteStateErr   error

	// DrainCallCount tracks the number of Drain calls
	DrainCallCount int
	// DrainErrOnCall specifies which call number (1-indexed) should return DrainErr
	// If 0, DrainErr applies to all calls. If >0, DrainErr only applies to that call number.
	DrainErrOnCall int
}

// NewMockStore creates a new MockStore with initialized maps.
func NewMockStore() *MockStore {
	return &MockStore{
		entries: map[staging.Service]map[staging.EntryKey]staging.Entry{
			staging.ServiceParam:  make(map[staging.EntryKey]staging.Entry),
			staging.ServiceSecret: make(map[staging.EntryKey]staging.Entry),
		},
		tags: map[staging.Service]map[staging.EntryKey]staging.TagEntry{
			staging.ServiceParam:  make(map[staging.EntryKey]staging.TagEntry),
			staging.ServiceSecret: make(map[staging.EntryKey]staging.TagEntry),
		},
	}
}

// GetEntry retrieves the staged entry identified by key.
func (m *MockStore) GetEntry(_ context.Context, service staging.Service, key staging.EntryKey) (*staging.Entry, error) {
	if m.GetEntryErr != nil {
		return nil, m.GetEntryErr
	}

	if entry, ok := m.entries[service][key]; ok {
		return &entry, nil
	}

	return nil, staging.ErrNotStaged
}

// GetTag retrieves the staged tag changes identified by key.
func (m *MockStore) GetTag(_ context.Context, service staging.Service, key staging.EntryKey) (*staging.TagEntry, error) {
	if m.GetTagErr != nil {
		return nil, m.GetTagErr
	}

	if tag, ok := m.tags[service][key]; ok {
		return &tag, nil
	}

	return nil, staging.ErrNotStaged
}

// StageEntry adds or updates the staged entry identified by key.
func (m *MockStore) StageEntry(_ context.Context, service staging.Service, key staging.EntryKey, entry staging.Entry) error {
	if m.StageEntryErr != nil {
		return m.StageEntryErr
	}

	m.entries[service][key] = entry

	return nil
}

// StageTag adds or updates the staged tag changes identified by key.
func (m *MockStore) StageTag(_ context.Context, service staging.Service, key staging.EntryKey, tag staging.TagEntry) error {
	if m.StageTagErr != nil {
		return m.StageTagErr
	}

	m.tags[service][key] = tag

	return nil
}

// UnstageEntry removes the staged entry identified by key.
func (m *MockStore) UnstageEntry(_ context.Context, service staging.Service, key staging.EntryKey) error {
	if m.UnstageEntryErr != nil {
		return m.UnstageEntryErr
	}

	if _, ok := m.entries[service][key]; !ok {
		return staging.ErrNotStaged
	}

	delete(m.entries[service], key)

	return nil
}

// UnstageTag removes the staged tag changes identified by key.
func (m *MockStore) UnstageTag(_ context.Context, service staging.Service, key staging.EntryKey) error {
	if m.UnstageTagErr != nil {
		return m.UnstageTagErr
	}

	if _, ok := m.tags[service][key]; !ok {
		return staging.ErrNotStaged
	}

	delete(m.tags[service], key)

	return nil
}

// UnstageAll removes all staged changes for a service.
func (m *MockStore) UnstageAll(_ context.Context, service staging.Service) error {
	if m.UnstageAllErr != nil {
		return m.UnstageAllErr
	}

	for _, svc := range m.servicesFor(service) {
		m.entries[svc] = make(map[staging.EntryKey]staging.Entry)
		m.tags[svc] = make(map[staging.EntryKey]staging.TagEntry)
	}

	return nil
}

// servicesFor returns the services to operate on for the given service filter.
// An empty filter expands to the services the mock tracks, mirroring the real
// store's scope-driven iteration instead of hardcoding {param, secret}.
func (m *MockStore) servicesFor(service staging.Service) []staging.Service {
	if service != "" {
		return []staging.Service{service}
	}

	return slices.Sorted(maps.Keys(m.entries))
}

// ListEntries returns all staged entries for a service.
func (m *MockStore) ListEntries(_ context.Context, service staging.Service) (map[staging.Service]map[staging.EntryKey]staging.Entry, error) {
	if m.ListEntriesErr != nil {
		return nil, m.ListEntriesErr
	}

	result := make(map[staging.Service]map[staging.EntryKey]staging.Entry)

	for _, svc := range m.servicesFor(service) {
		if len(m.entries[svc]) > 0 {
			result[svc] = maps.Clone(m.entries[svc])
		}
	}

	return result, nil
}

// ListTags returns all staged tag changes for a service.
func (m *MockStore) ListTags(_ context.Context, service staging.Service) (map[staging.Service]map[staging.EntryKey]staging.TagEntry, error) {
	if m.ListTagsErr != nil {
		return nil, m.ListTagsErr
	}

	result := make(map[staging.Service]map[staging.EntryKey]staging.TagEntry)

	for _, svc := range m.servicesFor(service) {
		if len(m.tags[svc]) > 0 {
			result[svc] = maps.Clone(m.tags[svc])
		}
	}

	return result, nil
}

// AddEntry is a helper to add entries directly for testing.
func (m *MockStore) AddEntry(service staging.Service, key staging.EntryKey, entry staging.Entry) {
	m.entries[service][key] = entry
}

// AddTag is a helper to add tag entries directly for testing.
func (m *MockStore) AddTag(service staging.Service, key staging.EntryKey, tag staging.TagEntry) {
	m.tags[service][key] = tag
}

// Drain retrieves the entire state from storage.
// If service is empty, returns all services; otherwise filters to the specified service.
// If keep is false, the source storage is cleared after reading.
func (m *MockStore) Drain(_ context.Context, service staging.Service, keep bool) (*staging.State, error) {
	m.DrainCallCount++
	if m.DrainErr != nil {
		// If DrainErrOnCall is specified, only return error on that specific call
		if m.DrainErrOnCall == 0 || m.DrainErrOnCall == m.DrainCallCount {
			return nil, m.DrainErr
		}
	}

	// Copy current state
	state := staging.NewEmptyState()
	for svc, entries := range m.entries {
		maps.Copy(state.Entries[svc], entries)
	}

	for svc, tags := range m.tags {
		maps.Copy(state.Tags[svc], tags)
	}

	// Clear storage if not keeping
	if !keep {
		for _, svc := range m.servicesFor("") {
			m.entries[svc] = make(map[staging.EntryKey]staging.Entry)
			m.tags[svc] = make(map[staging.EntryKey]staging.TagEntry)
		}
	}

	// Filter by service if specified
	if service != "" {
		return state.ExtractService(service), nil
	}

	return state, nil
}

// WriteState writes the entire state to storage.
// If service is empty, writes all services; otherwise writes only the specified service.
func (m *MockStore) WriteState(_ context.Context, service staging.Service, state *staging.State) error {
	if m.WriteStateErr != nil {
		return m.WriteStateErr
	}

	// Filter by service if specified
	if service != "" {
		state = state.ExtractService(service)
	}

	// Replace all entries and tags
	for _, svc := range m.servicesFor("") {
		m.entries[svc] = make(map[staging.EntryKey]staging.Entry)
		m.tags[svc] = make(map[staging.EntryKey]staging.TagEntry)
	}

	if state == nil {
		return nil
	}

	for svc, entries := range state.Entries {
		maps.Copy(m.entries[svc], entries)
	}

	for svc, tags := range state.Tags {
		maps.Copy(m.tags[svc], tags)
	}

	return nil
}

// Compile-time checks that MockStore implements interfaces.
var (
	_ store.ReadWriteOperator = (*MockStore)(nil)
	_ store.FileStore         = (*MockStore)(nil)
)
