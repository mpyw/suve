// Package staging provides staging functionality for AWS parameter and secret changes.
package staging

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/mpyw/suve/internal/maputil"
)

// EntryKey identifies a staged item — an entry or a tag change — by name and
// namespace. Namespace is the Azure App Configuration label axis; empty is the
// null/default namespace and the only value for every other provider. Carrying
// both in one value is what makes it impossible to address a staged item
// without its namespace: the store API and the in-memory state maps are keyed
// by EntryKey, so a namespaced App Configuration setting can never be silently
// resolved under the default namespace. The same name under two namespaces is
// two distinct items.
type EntryKey struct {
	Name      string
	Namespace string
}

// Label renders the key for display, appending the namespace as a [badge] when
// present. The empty (default) namespace — the only value for AWS, Google Cloud
// and Key Vault — renders as the bare name, matching status/diff output.
func (k EntryKey) Label() string {
	if k.Namespace == "" {
		return k.Name
	}

	return fmt.Sprintf("%s [%s]", k.Name, k.Namespace)
}

// SortedEntryKeys returns the keys of m sorted by (name, namespace) for
// deterministic iteration and output.
func SortedEntryKeys[V any](m map[EntryKey]V) []EntryKey {
	keys := make([]EntryKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.SortFunc(keys, func(a, b EntryKey) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return strings.Compare(a.Namespace, b.Namespace)
	})

	return keys
}

// stateVersion is the current version of the staging state format. v3 replaced
// the NUL-composite string map keys of v2 with structured (name, namespace)
// records. Pre-v3 state is NOT migrated — the NUL encoding is gone entirely, so
// an older on-disk state is treated as empty (staging is local working state; a
// one-time reset on the format change is acceptable). See State.UnmarshalJSON.
const stateVersion = 3

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
	//nolint:tagliatelle // JSON uses snake_case for consistency with file storage format
	StagedAt time.Time `json:"staged_at"`
	// BaseModifiedAt records the AWS LastModified time when the value was fetched.
	// Used for conflict detection: if AWS was modified after this time, it's a conflict.
	// Only set for update/delete operations (nil for create since there's no base).
	//nolint:tagliatelle // JSON uses snake_case for consistency with file storage format
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
	// DeleteOptions holds Secrets Manager-specific delete options.
	// Only used when Operation is OperationDelete and service is Secrets Manager.
	//nolint:tagliatelle // JSON uses snake_case for consistency with file storage format
	DeleteOptions *DeleteOptions `json:"delete_options,omitempty"`
}

// TagEntry represents staged tag changes for an entity.
// Managed separately from Entry for cleaner separation of concerns.
type TagEntry struct {
	Add    map[string]string   `json:"add,omitempty"`    // Tags to add or update
	Remove maputil.Set[string] `json:"remove,omitempty"` // Tag keys to remove
	// StagedAt records when the tag change was staged.
	//nolint:tagliatelle // JSON uses snake_case for consistency with file storage format
	StagedAt time.Time `json:"staged_at"`
	// BaseModifiedAt records the AWS LastModified time when tags were fetched.
	// Used for conflict detection.
	//nolint:tagliatelle // JSON uses snake_case for consistency with file storage format
	BaseModifiedAt *time.Time `json:"base_modified_at,omitempty"`
}

// DeleteOptions holds options for Secrets Manager delete operations.
type DeleteOptions struct {
	// Force enables immediate permanent deletion without recovery window.
	Force bool `json:"force,omitempty"`
	// RecoveryWindow is the number of days before permanent deletion (7-30).
	// Only used when Force is false. 0 means default (30 days).
	//nolint:tagliatelle // JSON uses snake_case for consistency with file storage format
	RecoveryWindow int `json:"recovery_window,omitempty"`
}

// State represents the entire staging state (v3). Entries and Tags are keyed by
// EntryKey (name + namespace) and managed separately for cleaner separation of
// concerns. On disk each item is a structured record carrying its name and
// namespace explicitly; see MarshalJSON / UnmarshalJSON.
type State struct {
	Version int
	Entries map[Service]map[EntryKey]Entry
	Tags    map[Service]map[EntryKey]TagEntry
}

// entryRecord is the on-disk form of a staged entry: the Entry fields plus the
// explicit (name, namespace) identity that keys it. No NUL-composite string.
type entryRecord struct {
	Entry

	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// tagRecord is the on-disk form of staged tag changes, keyed by explicit
// (name, namespace).
type tagRecord struct {
	TagEntry

	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// stateJSON is the on-disk shape of State: per-service arrays of records.
type stateJSON struct {
	Version int                       `json:"version"`
	Entries map[Service][]entryRecord `json:"entries,omitempty"`
	Tags    map[Service][]tagRecord   `json:"tags,omitempty"`
}

// MarshalJSON writes the state as structured (name, namespace) records, sorted
// for deterministic output.
func (s *State) MarshalJSON() ([]byte, error) {
	out := stateJSON{Version: stateVersion}

	for svc, m := range s.Entries {
		if len(m) == 0 {
			continue
		}

		if out.Entries == nil {
			out.Entries = make(map[Service][]entryRecord)
		}

		recs := make([]entryRecord, 0, len(m))
		for _, k := range SortedEntryKeys(m) {
			recs = append(recs, entryRecord{Name: k.Name, Namespace: k.Namespace, Entry: m[k]})
		}

		out.Entries[svc] = recs
	}

	for svc, m := range s.Tags {
		if len(m) == 0 {
			continue
		}

		if out.Tags == nil {
			out.Tags = make(map[Service][]tagRecord)
		}

		recs := make([]tagRecord, 0, len(m))
		for _, k := range SortedEntryKeys(m) {
			recs = append(recs, tagRecord{Name: k.Name, Namespace: k.Namespace, TagEntry: m[k]})
		}

		out.Tags[svc] = recs
	}

	return json.Marshal(out)
}

// UnmarshalJSON reads a v3 (structured-record) state. Pre-v3 layouts
// (NUL-composite string map keys) are intentionally NOT migrated: they are
// treated as empty (see stateVersion) so a format bump never crashes commands —
// stale local working state is dropped rather than converted.
func (s *State) UnmarshalJSON(data []byte) error {
	var head struct {
		Version int `json:"version"`
	}

	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}

	if head.Version < stateVersion {
		// Pre-v3 state used a different on-disk layout (NUL-composite string map
		// keys) that is intentionally not migrated. Treat it as empty so a format
		// bump never crashes commands; stale local working state is dropped.
		*s = *NewEmptyState()

		return nil
	}

	var in stateJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}

	s.Version = stateVersion
	s.Entries = make(map[Service]map[EntryKey]Entry)
	s.Tags = make(map[Service]map[EntryKey]TagEntry)

	for svc, recs := range in.Entries {
		m := make(map[EntryKey]Entry, len(recs))
		for _, rec := range recs {
			m[EntryKey{Name: rec.Name, Namespace: rec.Namespace}] = rec.Entry
		}

		s.Entries[svc] = m
	}

	for svc, recs := range in.Tags {
		m := make(map[EntryKey]TagEntry, len(recs))
		for _, rec := range recs {
			m[EntryKey{Name: rec.Name, Namespace: rec.Namespace}] = rec.TagEntry
		}

		s.Tags[svc] = m
	}

	return nil
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
		s.Entries = make(map[Service]map[EntryKey]Entry)
	}

	if s.Tags == nil {
		s.Tags = make(map[Service]map[EntryKey]TagEntry)
	}

	for svc, entries := range other.Entries {
		if s.Entries[svc] == nil {
			s.Entries[svc] = make(map[EntryKey]Entry)
		}

		maps.Copy(s.Entries[svc], entries)
	}

	for svc, tags := range other.Tags {
		if s.Tags[svc] == nil {
			s.Tags[svc] = make(map[EntryKey]TagEntry)
		}

		for key, incoming := range tags {
			if existing, ok := s.Tags[svc][key]; ok {
				s.Tags[svc][key] = mergeTagEntry(existing, incoming)
			} else {
				s.Tags[svc][key] = incoming
			}
		}
	}
}

// mergeTagEntry field-unions two staged tag deltas for the same key instead of
// replacing base wholesale. Value entries are complete snapshots and stay
// wholesale-replaced, but tag deltas are partial edits, so replacing base
// wholesale silently drops the base side's pending Add/Remove. The incoming
// (envelope) side wins per tag-key: its Add value overrides, and an
// Add-vs-Remove clash on the same tag-key resolves to whichever side incoming
// chose. Tag-keys only present in base are preserved (union). Non-tag fields
// (StagedAt, BaseModifiedAt) follow the winning envelope.
func mergeTagEntry(base, incoming TagEntry) TagEntry {
	tagKeys := maputil.NewSet[string]()
	for k := range base.Add {
		tagKeys.Add(k)
	}

	for k := range incoming.Add {
		tagKeys.Add(k)
	}

	for k := range base.Remove {
		tagKeys.Add(k)
	}

	for k := range incoming.Remove {
		tagKeys.Add(k)
	}

	add := make(map[string]string)
	remove := make(maputil.Set[string])

	for k := range tagKeys {
		switch {
		case hasKey(incoming.Add, k):
			add[k] = incoming.Add[k]
		case incoming.Remove.Contains(k):
			remove.Add(k)
		case hasKey(base.Add, k):
			add[k] = base.Add[k]
		case base.Remove.Contains(k):
			remove.Add(k)
		}
	}

	merged := incoming

	if len(add) == 0 {
		add = nil
	}

	if len(remove) == 0 {
		remove = nil
	}

	merged.Add = add
	merged.Remove = remove

	return merged
}

// hasKey reports whether m contains key, distinguishing an explicit empty-string
// tag value from an absent tag-key.
func hasKey(m map[string]string, key string) bool {
	_, ok := m[key]

	return ok
}

// NewEmptyState creates a new empty state with initialized maps.
func NewEmptyState() *State {
	return &State{
		Version: stateVersion,
		Entries: map[Service]map[EntryKey]Entry{
			ServiceParam:  make(map[EntryKey]Entry),
			ServiceSecret: make(map[EntryKey]Entry),
		},
		Tags: map[Service]map[EntryKey]TagEntry{
			ServiceParam:  make(map[EntryKey]TagEntry),
			ServiceSecret: make(map[EntryKey]TagEntry),
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
		s.Entries = map[Service]map[EntryKey]Entry{
			ServiceParam:  make(map[EntryKey]Entry),
			ServiceSecret: make(map[EntryKey]Entry),
		}
		s.Tags = map[Service]map[EntryKey]TagEntry{
			ServiceParam:  make(map[EntryKey]TagEntry),
			ServiceSecret: make(map[EntryKey]TagEntry),
		}

		return
	}

	s.Entries[service] = make(map[EntryKey]Entry)
	s.Tags[service] = make(map[EntryKey]TagEntry)
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
