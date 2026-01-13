// Package testutil provides test utilities for staging package.
package testutil

import (
	"context"

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

// Compile-time check that MockStore implements ReadWriteOperator.
var _ store.ReadWriteOperator = (*MockStore)(nil)
