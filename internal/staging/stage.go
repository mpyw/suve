// Package staging provides staging functionality for AWS parameter and secret changes.
package staging

import (
	"errors"
	"maps"
	"time"

	"github.com/mpyw/suve/internal/maputil"
)

// Operation represents the type of staged change.
type Operation string

const (
	// OperationCreate represents a create operation (new item).
	OperationCreate Operation = "create"
	// OperationUpdate represents an update operation (existing item).
	OperationUpdate Operation = "update"
	// OperationDelete represents a delete operation.
	OperationDelete Operation = "delete"
)

// Entry represents a single staged entity change (create/update/delete).
// Tags are managed separately in TagEntry.
type Entry struct {
	Operation   Operation `json:"operation"`
	Value       *string   `json:"value,omitempty"` // nil for delete, pointer to distinguish from empty string
	Description *string   `json:"description,omitempty"`
	StagedAt    time.Time `json:"staged_at"`
	// BaseModifiedAt records the AWS LastModified time when the value was fetched.
	// Used for conflict detection: if AWS was modified after this time, it's a conflict.
	// Only set for update/delete operations (nil for create since there's no base).
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
	// DeleteOptions holds Secrets Manager-specific delete options.
	// Only used when Operation is OperationDelete and service is Secrets Manager.
	DeleteOptions *DeleteOptions `json:"delete_options,omitempty"`
}

// TagEntry represents staged tag changes for an entity.
// Managed separately from Entry for cleaner separation of concerns.
type TagEntry struct {
	Add    map[string]string   `json:"add,omitempty"`    // Tags to add or update
	Remove maputil.Set[string] `json:"remove,omitempty"` // Tag keys to remove
	// StagedAt records when the tag change was staged.
	StagedAt time.Time `json:"staged_at"`
	// BaseModifiedAt records the AWS LastModified time when tags were fetched.
	// Used for conflict detection.
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
}

// DeleteOptions holds options for Secrets Manager delete operations.
type DeleteOptions struct {
	// Force enables immediate permanent deletion without recovery window.
	Force bool `json:"force,omitempty"`
	// RecoveryWindow is the number of days before permanent deletion (7-30).
	// Only used when Force is false. 0 means default (30 days).
	RecoveryWindow int `json:"recovery_window,omitempty"`
}

// State represents the entire staging state (v2).
// Entries and Tags are managed separately for cleaner separation of concerns.
type State struct {
	Version int                             `json:"version"`
	Entries map[Service]map[string]Entry    `json:"entries,omitempty"`
	Tags    map[Service]map[string]TagEntry `json:"tags,omitempty"`
}

// IsEmpty checks if a state has no entries and no tags.
func (s *State) IsEmpty() bool {
	if s == nil {
		return true
	}
	for _, entries := range s.Entries {
		if len(entries) > 0 {
			return false
		}
	}
	for _, tags := range s.Tags {
		if len(tags) > 0 {
			return false
		}
	}
	return true
}

// TotalCount returns the total number of entries and tags in the state.
func (s *State) TotalCount() int {
	if s == nil {
		return 0
	}
	return s.EntryCount() + s.TagCount()
}

// EntryCount returns the total number of entries in the state.
func (s *State) EntryCount() int {
	if s == nil {
		return 0
	}
	count := 0
	for _, entries := range s.Entries {
		count += len(entries)
	}
	return count
}

// TagCount returns the total number of tag entries in the state.
func (s *State) TagCount() int {
	if s == nil {
		return 0
	}
	count := 0
	for _, tags := range s.Tags {
		count += len(tags)
	}
	return count
}

// Merge merges another state into this state.
// The other state takes precedence for conflicting entries.
func (s *State) Merge(other *State) {
	if other == nil {
		return
	}
	if s.Entries == nil {
		s.Entries = make(map[Service]map[string]Entry)
	}
	if s.Tags == nil {
		s.Tags = make(map[Service]map[string]TagEntry)
	}

	for svc, entries := range other.Entries {
		if s.Entries[svc] == nil {
			s.Entries[svc] = make(map[string]Entry)
		}
		maps.Copy(s.Entries[svc], entries)
	}
	for svc, tags := range other.Tags {
		if s.Tags[svc] == nil {
			s.Tags[svc] = make(map[string]TagEntry)
		}
		maps.Copy(s.Tags[svc], tags)
	}
}

// NewEmptyState creates a new empty state with initialized maps.
func NewEmptyState() *State {
	return &State{
		Version: 2,
		Entries: map[Service]map[string]Entry{
			ServiceParam:  make(map[string]Entry),
			ServiceSecret: make(map[string]Entry),
		},
		Tags: map[Service]map[string]TagEntry{
			ServiceParam:  make(map[string]TagEntry),
			ServiceSecret: make(map[string]TagEntry),
		},
	}
}

// ExtractService returns a new state containing only entries for the specified service.
// If service is empty, returns a clone of the entire state.
func (s *State) ExtractService(service Service) *State {
	if s == nil {
		return NewEmptyState()
	}
	if service == "" {
		// Clone entire state
		result := NewEmptyState()
		result.Merge(s)
		return result
	}
	result := NewEmptyState()
	if entries, ok := s.Entries[service]; ok {
		maps.Copy(result.Entries[service], entries)
	}
	if tags, ok := s.Tags[service]; ok {
		maps.Copy(result.Tags[service], tags)
	}
	return result
}

// RemoveService removes all entries for the specified service from this state.
// If service is empty, clears all entries.
func (s *State) RemoveService(service Service) {
	if s == nil {
		return
	}
	if service == "" {
		// Clear all
		s.Entries = map[Service]map[string]Entry{
			ServiceParam:  make(map[string]Entry),
			ServiceSecret: make(map[string]Entry),
		}
		s.Tags = map[Service]map[string]TagEntry{
			ServiceParam:  make(map[string]TagEntry),
			ServiceSecret: make(map[string]TagEntry),
		}
		return
	}
	s.Entries[service] = make(map[string]Entry)
	s.Tags[service] = make(map[string]TagEntry)
}

// Service represents which AWS service the staged change belongs to.
type Service string

const (
	// ServiceParam represents AWS Systems Manager Parameter Store.
	ServiceParam Service = "param"
	// ServiceSecret represents AWS Secrets Manager.
	ServiceSecret Service = "secret"
)

var (
	// ErrNotStaged is returned when a parameter/secret is not staged.
	ErrNotStaged = errors.New("not staged")
)
