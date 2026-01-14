// Package testutil provides test utilities for staging package.
package testutil

import (
	"context"
	"maps"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// MockStore implements store.ReadWriteOperator for testing.
// It stores state in memory and can be configured to return errors.
type MockStore struct {
	entries map[staging.Service]map[string]staging.Entry
	tags    map[staging.Service]map[string]staging.TagEntry

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
	PingErr         error
	StartErr        error

	// DrainCallCount tracks the number of Drain calls
	DrainCallCount int
	// DrainErrOnCall specifies which call number (1-indexed) should return DrainErr
	// If 0, DrainErr applies to all calls. If >0, DrainErr only applies to that call number.
	DrainErrOnCall int
}

// NewMockStore creates a new MockStore with initialized maps.
func NewMockStore() *MockStore {
	return &MockStore{
		entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  make(map[string]staging.Entry),
			staging.ServiceSecret: make(map[string]staging.Entry),
		},
		tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  make(map[string]staging.TagEntry),
			staging.ServiceSecret: make(map[string]staging.TagEntry),
		},
	}
}

// GetEntry retrieves a staged entry.
func (m *MockStore) GetEntry(_ context.Context, service staging.Service, name string) (*staging.Entry, error) {
	if m.GetEntryErr != nil {
		return nil, m.GetEntryErr
	}

	if entry, ok := m.entries[service][name]; ok {
		return &entry, nil
	}

	return nil, staging.ErrNotStaged
}

// GetTag retrieves staged tag changes.
func (m *MockStore) GetTag(_ context.Context, service staging.Service, name string) (*staging.TagEntry, error) {
	if m.GetTagErr != nil {
		return nil, m.GetTagErr
	}

	if tag, ok := m.tags[service][name]; ok {
		return &tag, nil
	}

	return nil, staging.ErrNotStaged
}

// StageEntry adds or updates a staged entry change.
func (m *MockStore) StageEntry(_ context.Context, service staging.Service, name string, entry staging.Entry) error {
	if m.StageEntryErr != nil {
		return m.StageEntryErr
	}

	m.entries[service][name] = entry

	return nil
}

// StageTag adds or updates staged tag changes.
func (m *MockStore) StageTag(_ context.Context, service staging.Service, name string, tag staging.TagEntry) error {
	if m.StageTagErr != nil {
		return m.StageTagErr
	}

	m.tags[service][name] = tag

	return nil
}

// UnstageEntry removes a staged entry change.
func (m *MockStore) UnstageEntry(_ context.Context, service staging.Service, name string) error {
	if m.UnstageEntryErr != nil {
		return m.UnstageEntryErr
	}

	if _, ok := m.entries[service][name]; !ok {
		return staging.ErrNotStaged
	}

	delete(m.entries[service], name)

	return nil
}

// UnstageTag removes staged tag changes.
func (m *MockStore) UnstageTag(_ context.Context, service staging.Service, name string) error {
	if m.UnstageTagErr != nil {
		return m.UnstageTagErr
	}

	if _, ok := m.tags[service][name]; !ok {
		return staging.ErrNotStaged
	}

	delete(m.tags[service], name)

	return nil
}

// UnstageAll removes all staged changes for a service.
func (m *MockStore) UnstageAll(_ context.Context, service staging.Service) error {
	if m.UnstageAllErr != nil {
		return m.UnstageAllErr
	}

	switch service {
	case staging.ServiceParam:
		m.entries[staging.ServiceParam] = make(map[string]staging.Entry)
		m.tags[staging.ServiceParam] = make(map[string]staging.TagEntry)
	case staging.ServiceSecret:
		m.entries[staging.ServiceSecret] = make(map[string]staging.Entry)
		m.tags[staging.ServiceSecret] = make(map[string]staging.TagEntry)
	case "":
		m.entries[staging.ServiceParam] = make(map[string]staging.Entry)
		m.entries[staging.ServiceSecret] = make(map[string]staging.Entry)
		m.tags[staging.ServiceParam] = make(map[string]staging.TagEntry)
		m.tags[staging.ServiceSecret] = make(map[string]staging.TagEntry)
	}

	return nil
}

// ListEntries returns all staged entries for a service.
//
//nolint:dupl // similar structure to ListTags but different types
func (m *MockStore) ListEntries(_ context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
	if m.ListEntriesErr != nil {
		return nil, m.ListEntriesErr
	}

	result := make(map[staging.Service]map[string]staging.Entry)

	switch service {
	case staging.ServiceParam:
		if len(m.entries[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = m.entries[staging.ServiceParam]
		}
	case staging.ServiceSecret:
		if len(m.entries[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = m.entries[staging.ServiceSecret]
		}
	case "":
		if len(m.entries[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = m.entries[staging.ServiceParam]
		}

		if len(m.entries[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = m.entries[staging.ServiceSecret]
		}
	}

	return result, nil
}

// ListTags returns all staged tag changes for a service.
//
//nolint:dupl // similar structure to ListEntries but different types
func (m *MockStore) ListTags(_ context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	if m.ListTagsErr != nil {
		return nil, m.ListTagsErr
	}

	result := make(map[staging.Service]map[string]staging.TagEntry)

	switch service {
	case staging.ServiceParam:
		if len(m.tags[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = m.tags[staging.ServiceParam]
		}
	case staging.ServiceSecret:
		if len(m.tags[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = m.tags[staging.ServiceSecret]
		}
	case "":
		if len(m.tags[staging.ServiceParam]) > 0 {
			result[staging.ServiceParam] = m.tags[staging.ServiceParam]
		}

		if len(m.tags[staging.ServiceSecret]) > 0 {
			result[staging.ServiceSecret] = m.tags[staging.ServiceSecret]
		}
	}

	return result, nil
}

// AddEntry is a helper to add entries directly for testing.
func (m *MockStore) AddEntry(service staging.Service, name string, entry staging.Entry) {
	m.entries[service][name] = entry
}

// AddTag is a helper to add tag entries directly for testing.
func (m *MockStore) AddTag(service staging.Service, name string, tag staging.TagEntry) {
	m.tags[service][name] = tag
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
		m.entries = map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  make(map[string]staging.Entry),
			staging.ServiceSecret: make(map[string]staging.Entry),
		}
		m.tags = map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  make(map[string]staging.TagEntry),
			staging.ServiceSecret: make(map[string]staging.TagEntry),
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
	m.entries = map[staging.Service]map[string]staging.Entry{
		staging.ServiceParam:  make(map[string]staging.Entry),
		staging.ServiceSecret: make(map[string]staging.Entry),
	}
	m.tags = map[staging.Service]map[string]staging.TagEntry{
		staging.ServiceParam:  make(map[string]staging.TagEntry),
		staging.ServiceSecret: make(map[string]staging.TagEntry),
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

// Ping checks if the agent daemon is running.
func (m *MockStore) Ping(_ context.Context) error {
	return m.PingErr
}

// Start ensures the agent daemon is running, starting it if necessary.
func (m *MockStore) Start(_ context.Context) error {
	return m.StartErr
}

// HintedMockStore extends MockStore with HintedUnstager support.
type HintedMockStore struct {
	*MockStore

	UnstageEntryWithHintErr error
	UnstageTagWithHintErr   error
	UnstageAllWithHintErr   error
	LastHint                string // Records the last hint used
}

// NewHintedMockStore creates a new HintedMockStore with initialized maps.
func NewHintedMockStore() *HintedMockStore {
	return &HintedMockStore{
		MockStore: NewMockStore(),
	}
}

// UnstageEntryWithHint removes a staged entry with an operation hint.
func (m *HintedMockStore) UnstageEntryWithHint(ctx context.Context, service staging.Service, name string, hint string) error {
	m.LastHint = hint
	if m.UnstageEntryWithHintErr != nil {
		return m.UnstageEntryWithHintErr
	}

	return m.UnstageEntry(ctx, service, name)
}

// UnstageTagWithHint removes staged tag changes with an operation hint.
func (m *HintedMockStore) UnstageTagWithHint(ctx context.Context, service staging.Service, name string, hint string) error {
	m.LastHint = hint
	if m.UnstageTagWithHintErr != nil {
		return m.UnstageTagWithHintErr
	}

	return m.UnstageTag(ctx, service, name)
}

// UnstageAllWithHint removes all staged changes with an operation hint.
func (m *HintedMockStore) UnstageAllWithHint(ctx context.Context, service staging.Service, hint string) error {
	m.LastHint = hint
	if m.UnstageAllWithHintErr != nil {
		return m.UnstageAllWithHintErr
	}

	return m.UnstageAll(ctx, service)
}

// MockServiceStore wraps MockStore for a specific service, implementing ServiceStore.
type MockServiceStore struct {
	parent  *MockStore
	service staging.Service
}

// ForService creates a service-scoped MockServiceStore.
func (m *MockStore) ForService(service staging.Service) *MockServiceStore {
	return &MockServiceStore{
		parent:  m,
		service: service,
	}
}

// Service creates a service-scoped store (implements AgentStoreFactory).
func (m *MockStore) Service(service staging.Service) store.ServiceStore {
	return m.ForService(service)
}

// GetEntry retrieves a staged entry by name.
func (s *MockServiceStore) GetEntry(ctx context.Context, name string) (*staging.Entry, error) {
	return s.parent.GetEntry(ctx, s.service, name)
}

// GetTag retrieves staged tag changes by name.
func (s *MockServiceStore) GetTag(ctx context.Context, name string) (*staging.TagEntry, error) {
	return s.parent.GetTag(ctx, s.service, name)
}

// ListEntries returns all staged entries for this service.
func (s *MockServiceStore) ListEntries(ctx context.Context) (map[string]staging.Entry, error) {
	all, err := s.parent.ListEntries(ctx, s.service)
	if err != nil {
		return nil, err
	}

	entries := all[s.service]
	if entries == nil {
		entries = make(map[string]staging.Entry)
	}

	return entries, nil
}

// ListTags returns all staged tag changes for this service.
func (s *MockServiceStore) ListTags(ctx context.Context) (map[string]staging.TagEntry, error) {
	all, err := s.parent.ListTags(ctx, s.service)
	if err != nil {
		return nil, err
	}

	tags := all[s.service]
	if tags == nil {
		tags = make(map[string]staging.TagEntry)
	}

	return tags, nil
}

// StageEntry adds or updates a staged entry.
func (s *MockServiceStore) StageEntry(ctx context.Context, name string, entry staging.Entry) error {
	return s.parent.StageEntry(ctx, s.service, name, entry)
}

// StageTag adds or updates staged tag changes.
func (s *MockServiceStore) StageTag(ctx context.Context, name string, tagEntry staging.TagEntry) error {
	return s.parent.StageTag(ctx, s.service, name, tagEntry)
}

// UnstageEntry removes a staged entry.
func (s *MockServiceStore) UnstageEntry(ctx context.Context, name string) error {
	return s.parent.UnstageEntry(ctx, s.service, name)
}

// UnstageTag removes staged tag changes.
func (s *MockServiceStore) UnstageTag(ctx context.Context, name string) error {
	return s.parent.UnstageTag(ctx, s.service, name)
}

// UnstageAll removes all staged changes for this service.
func (s *MockServiceStore) UnstageAll(ctx context.Context) error {
	return s.parent.UnstageAll(ctx, s.service)
}

// Drain retrieves state for this service.
func (s *MockServiceStore) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	return s.parent.Drain(ctx, s.service, keep)
}

// WriteState writes state for this service.
func (s *MockServiceStore) WriteState(ctx context.Context, state *staging.State) error {
	return s.parent.WriteState(ctx, s.service, state)
}

// HintedMockServiceStore wraps HintedMockStore for a specific service.
type HintedMockServiceStore struct {
	*MockServiceStore

	hintedParent *HintedMockStore
}

// ForService creates a service-scoped HintedMockServiceStore.
func (m *HintedMockStore) ForService(service staging.Service) *HintedMockServiceStore {
	return &HintedMockServiceStore{
		MockServiceStore: m.MockStore.ForService(service),
		hintedParent:     m,
	}
}

// UnstageEntryWithHint removes a staged entry with an operation hint.
func (s *HintedMockServiceStore) UnstageEntryWithHint(ctx context.Context, name string, hint string) error {
	return s.hintedParent.UnstageEntryWithHint(ctx, s.service, name, hint)
}

// UnstageTagWithHint removes staged tag changes with an operation hint.
func (s *HintedMockServiceStore) UnstageTagWithHint(ctx context.Context, name string, hint string) error {
	return s.hintedParent.UnstageTagWithHint(ctx, s.service, name, hint)
}

// UnstageAllWithHint removes all staged changes with an operation hint.
func (s *HintedMockServiceStore) UnstageAllWithHint(ctx context.Context, hint string) error {
	return s.hintedParent.UnstageAllWithHint(ctx, s.service, hint)
}

// MockGlobalStore wraps MockStore for global operations, implementing GlobalStore.
type MockGlobalStore struct {
	parent *MockStore
}

// Global creates a MockGlobalStore that implements store.GlobalStore (implements AgentStoreFactory).
func (m *MockStore) Global() store.GlobalStore {
	return &MockGlobalStore{parent: m}
}

// ListEntries returns all staged entries across all services.
func (g *MockGlobalStore) ListEntries(ctx context.Context) (map[staging.Service]map[string]staging.Entry, error) {
	return g.parent.ListEntries(ctx, "")
}

// ListTags returns all staged tags across all services.
func (g *MockGlobalStore) ListTags(ctx context.Context) (map[staging.Service]map[string]staging.TagEntry, error) {
	return g.parent.ListTags(ctx, "")
}

// Drain retrieves state across all services.
func (g *MockGlobalStore) Drain(ctx context.Context, keep bool) (*staging.State, error) {
	return g.parent.Drain(ctx, "", keep)
}

// WriteState writes state across all services.
func (g *MockGlobalStore) WriteState(ctx context.Context, state *staging.State) error {
	return g.parent.WriteState(ctx, "", state)
}

// UnstageAll removes all staged changes across all services.
func (g *MockGlobalStore) UnstageAll(ctx context.Context) error {
	return g.parent.UnstageAll(ctx, "")
}

// HintedMockGlobalStore wraps HintedMockStore for global operations with hint support.
type HintedMockGlobalStore struct {
	*MockGlobalStore

	hintedParent *HintedMockStore
}

// Service creates a service-scoped store with hint support (implements AgentStoreFactory).
func (m *HintedMockStore) Service(service staging.Service) store.ServiceStore {
	return &HintedMockServiceStore{
		MockServiceStore: &MockServiceStore{
			parent:  m.MockStore,
			service: service,
		},
		hintedParent: m,
	}
}

// Global creates a HintedMockGlobalStore that implements store.GlobalStore and HintedGlobalUnstager.
func (m *HintedMockStore) Global() store.GlobalStore {
	return &HintedMockGlobalStore{
		MockGlobalStore: &MockGlobalStore{parent: m.MockStore},
		hintedParent:    m,
	}
}

// UnstageAllWithHint removes all staged changes with an operation hint.
func (g *HintedMockGlobalStore) UnstageAllWithHint(ctx context.Context, hint string) error {
	return g.hintedParent.UnstageAllWithHint(ctx, "", hint)
}

// Compile-time checks that MockStore implements interfaces.
//
//nolint:staticcheck // intentionally using legacy interfaces during migration
var (
	_ store.ReadWriteOperator     = (*MockStore)(nil)
	_ store.FileStore             = (*MockStore)(nil)
	_ store.AgentStore            = (*MockStore)(nil)
	_ store.AgentStoreFactory     = (*MockStore)(nil)
	_ store.AgentStoreFactory     = (*HintedMockStore)(nil)
	_ store.HintedUnstager        = (*HintedMockStore)(nil)
	_ store.ServiceStore          = (*MockServiceStore)(nil)
	_ store.ServiceReadWriter     = (*MockServiceStore)(nil)
	_ store.ServiceReadWriter     = (*HintedMockServiceStore)(nil)
	_ store.HintedServiceUnstager = (*HintedMockServiceStore)(nil)
	_ store.GlobalStore           = (*MockGlobalStore)(nil)
	_ store.DrainWriter           = (*MockGlobalStore)(nil)
	_ store.GlobalStore           = (*HintedMockGlobalStore)(nil)
	_ store.HintedGlobalUnstager  = (*HintedMockGlobalStore)(nil)
)
