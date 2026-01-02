package staging_test

import (
	"github.com/mpyw/suve/internal/staging"
)

// mockStore implements staging.StoreReadWriter for testing.
type mockStore struct {
	entries       map[staging.Service]map[string]staging.Entry
	getErr        error
	listErr       error
	stageErr      error
	unstageErr    error
	unstageAllErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}
}

func (m *mockStore) Get(service staging.Service, name string) (*staging.Entry, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if entries, ok := m.entries[service]; ok {
		if entry, ok := entries[name]; ok {
			return &entry, nil
		}
	}
	return nil, staging.ErrNotStaged
}

func (m *mockStore) List(service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	result := make(map[staging.Service]map[string]staging.Entry)
	if service == "" {
		for s, entries := range m.entries {
			if len(entries) > 0 {
				result[s] = entries
			}
		}
	} else if entries, ok := m.entries[service]; ok && len(entries) > 0 {
		result[service] = entries
	}
	return result, nil
}

func (m *mockStore) Stage(service staging.Service, name string, entry staging.Entry) error {
	if m.stageErr != nil {
		return m.stageErr
	}
	if _, ok := m.entries[service]; !ok {
		m.entries[service] = make(map[string]staging.Entry)
	}
	m.entries[service][name] = entry
	return nil
}

func (m *mockStore) Unstage(service staging.Service, name string) error {
	if m.unstageErr != nil {
		return m.unstageErr
	}
	if entries, ok := m.entries[service]; ok {
		if _, ok := entries[name]; ok {
			delete(entries, name)
			return nil
		}
	}
	return staging.ErrNotStaged
}

func (m *mockStore) UnstageAll(service staging.Service) error {
	if m.unstageAllErr != nil {
		return m.unstageAllErr
	}
	if service == "" {
		m.entries = map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		}
	} else {
		m.entries[service] = make(map[string]staging.Entry)
	}
	return nil
}

// Helper to add entries directly for testing
func (m *mockStore) addEntry(service staging.Service, name string, entry staging.Entry) {
	if _, ok := m.entries[service]; !ok {
		m.entries[service] = make(map[string]staging.Entry)
	}
	m.entries[service][name] = entry
}
