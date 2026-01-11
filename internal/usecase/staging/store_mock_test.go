package staging_test

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
)

// mockStore implements staging.StoreReadWriter for testing.
type mockStore struct {
	entries       map[staging.Service]map[string]staging.Entry
	tags          map[staging.Service]map[string]staging.TagEntry
	getErr        error
	getTagErr     error
	listErr       error
	listTagsErr   error
	stageErr      error
	stageTagErr   error
	unstageErr    error
	unstageTagErr error
	unstageAllErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
		tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}
}

func (m *mockStore) GetEntry(_ context.Context, service staging.Service, name string) (*staging.Entry, error) {
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

func (m *mockStore) GetTag(_ context.Context, service staging.Service, name string) (*staging.TagEntry, error) {
	if m.getTagErr != nil {
		return nil, m.getTagErr
	}
	if tags, ok := m.tags[service]; ok {
		if tag, ok := tags[name]; ok {
			return &tag, nil
		}
	}
	return nil, staging.ErrNotStaged
}

func (m *mockStore) ListEntries(_ context.Context, service staging.Service) (map[staging.Service]map[string]staging.Entry, error) {
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

func (m *mockStore) ListTags(_ context.Context, service staging.Service) (map[staging.Service]map[string]staging.TagEntry, error) {
	if m.listTagsErr != nil {
		return nil, m.listTagsErr
	}
	result := make(map[staging.Service]map[string]staging.TagEntry)
	if service == "" {
		for s, tags := range m.tags {
			if len(tags) > 0 {
				result[s] = tags
			}
		}
	} else if tags, ok := m.tags[service]; ok && len(tags) > 0 {
		result[service] = tags
	}
	return result, nil
}

func (m *mockStore) StageEntry(_ context.Context, service staging.Service, name string, entry staging.Entry) error {
	if m.stageErr != nil {
		return m.stageErr
	}
	if _, ok := m.entries[service]; !ok {
		m.entries[service] = make(map[string]staging.Entry)
	}
	m.entries[service][name] = entry
	return nil
}

func (m *mockStore) StageTag(_ context.Context, service staging.Service, name string, tagEntry staging.TagEntry) error {
	if m.stageTagErr != nil {
		return m.stageTagErr
	}
	if _, ok := m.tags[service]; !ok {
		m.tags[service] = make(map[string]staging.TagEntry)
	}
	m.tags[service][name] = tagEntry
	return nil
}

func (m *mockStore) UnstageEntry(_ context.Context, service staging.Service, name string) error {
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

func (m *mockStore) UnstageTag(_ context.Context, service staging.Service, name string) error {
	if m.unstageTagErr != nil {
		return m.unstageTagErr
	}
	if tags, ok := m.tags[service]; ok {
		if _, ok := tags[name]; ok {
			delete(tags, name)
			return nil
		}
	}
	return staging.ErrNotStaged
}

func (m *mockStore) UnstageAll(_ context.Context, service staging.Service) error {
	if m.unstageAllErr != nil {
		return m.unstageAllErr
	}
	if service == "" {
		m.entries = map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		}
		m.tags = map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		}
	} else {
		m.entries[service] = make(map[string]staging.Entry)
		m.tags[service] = make(map[string]staging.TagEntry)
	}
	return nil
}

func (m *mockStore) UnstageAllEntries(_ context.Context, service staging.Service) error {
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

func (m *mockStore) UnstageAllTags(_ context.Context, service staging.Service) error {
	if m.unstageAllErr != nil {
		return m.unstageAllErr
	}
	if service == "" {
		m.tags = map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		}
	} else {
		m.tags[service] = make(map[string]staging.TagEntry)
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

func (m *mockStore) Load(_ context.Context) (*staging.State, error) {
	return &staging.State{
		Version: 2,
		Entries: m.entries,
		Tags:    m.tags,
	}, nil
}
