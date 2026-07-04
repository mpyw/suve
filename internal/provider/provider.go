// Package provider defines the provider-neutral storage seam: the interfaces
// and opaque reference types that every backend (AWS SSM, AWS Secrets Manager,
// and future providers) implements.
//
// It imports only internal/domain and the standard library; it has ZERO
// knowledge of any cloud SDK. AWS-specific concerns (ARNs, staging labels,
// version-id semantics) live behind these interfaces inside the AWS adapter.
package provider

import (
	"context"

	"github.com/mpyw/suve/internal/domain"
)

// VersionRef is an opaque reference to a specific version, produced by a
// provider (Resolve/History) and consumed by the same provider. The zero
// value denotes the latest/current version. It intentionally exposes no
// version-id or staging-label semantics to generic callers.
type VersionRef struct{ id string }

// NewVersionRef builds a VersionRef from a provider-internal id. For adapter use.
func NewVersionRef(id string) VersionRef { return VersionRef{id: id} }

// ID returns the provider-internal identifier ("" for latest). For adapter use.
func (r VersionRef) ID() string { return r.id }

// IsLatest reports whether the ref denotes the latest/current version.
func (r VersionRef) IsLatest() bool { return r.id == "" }

// Reader provides read access to a provider's entries.
type Reader interface {
	// Resolve parses a provider-specific version spec string (e.g. "#3~1",
	// "#abc123", ":AWSCURRENT" for AWS) and resolves it to an opaque VersionRef.
	Resolve(ctx context.Context, name, spec string) (VersionRef, error)
	// Get retrieves the entry at the given version ref. It returns a wrapped
	// ErrNotFound if the entry does not exist.
	Get(ctx context.Context, name string, ref VersionRef) (*domain.Entry, error)
	// History returns the version history for an entry, newest first.
	History(ctx context.Context, name string) ([]domain.Version, error)
	// List returns the names of all entries in the provider's namespace.
	List(ctx context.Context) ([]string, error)
}

// Writer provides write access to a provider's entries.
type Writer interface {
	// Create creates a new entry and returns the resulting version. It returns
	// a wrapped ErrAlreadyExists if an entry with the same name already exists
	// (it never overwrites).
	Create(ctx context.Context, name, value string, valueType domain.ValueType, description string) (domain.Version, error)
	// Put creates or updates an entry (upsert) and returns the resulting
	// version. Unlike Create it overwrites an existing entry.
	Put(ctx context.Context, name, value string, valueType domain.ValueType, description string) (domain.Version, error)
	// Delete removes an entry.
	Delete(ctx context.Context, name string) error
}

// Tagger provides tag mutation for a provider's entries.
type Tagger interface {
	// Tag adds or updates the given tags on an entry.
	Tag(ctx context.Context, name string, add map[string]string) error
	// Untag removes the tags with the given keys from an entry.
	Untag(ctx context.Context, name string, keys []string) error
}

// Store is the full provider contract for one service (e.g. AWS SSM or
// Secrets Manager). Providers may additionally implement the optional
// Restorer/Describer capabilities.
type Store interface {
	Reader
	Writer
	Tagger
}

// Restorer restores a soft-deleted entry (e.g. Secrets Manager). Optional.
type Restorer interface {
	// Restore cancels a pending deletion for an entry.
	Restore(ctx context.Context, name string) error
}

// Describer returns entry metadata without the value. Optional.
type Describer interface {
	// Describe returns an entry's metadata without fetching its value.
	Describe(ctx context.Context, name string) (*domain.Entry, error)
}
