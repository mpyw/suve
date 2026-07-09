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

// WriteOption is a provider-interpreted functional option for create/update
// operations. Providers define concrete option types (e.g. AWS param Tier or
// DataType) that satisfy this marker by embedding WriteOptionMarker.
//
// Consumers (usecases, CLI) build and pass these options through WITHOUT
// type-asserting them; the provider adapter type-switches over the options it
// understands and silently ignores the rest. This keeps provider-specific
// options out of the neutral domain model while remaining strongly typed.
//
// The marker method is unexported, so the WriteOption set stays closed to types
// that embed WriteOptionMarker; arbitrary external types cannot masquerade as
// options.
type WriteOption interface{ writeOption() }

// WriteOptionMarker is embedded by provider-specific option types to satisfy
// WriteOption. Embedding it (rather than defining the unexported method in
// each provider package, which Go forbids across packages) is what lets the
// AWS adapters declare their own option types against this sealed interface.
type WriteOptionMarker struct{}

func (WriteOptionMarker) writeOption() {}

// DeleteOption is a provider-interpreted functional option for delete
// operations (e.g. AWS Secrets Manager ForceDelete / RecoveryWindow). It
// follows the same pass-through contract as WriteOption: consumers pass options
// through untyped and the adapter interprets the ones it recognizes. Concrete
// options satisfy it by embedding DeleteOptionMarker.
type DeleteOption interface{ deleteOption() }

// DeleteOptionMarker is embedded by provider-specific delete-option types to
// satisfy DeleteOption. See WriteOptionMarker for the rationale.
type DeleteOptionMarker struct{}

func (DeleteOptionMarker) deleteOption() {}

// ForceDelete requests immediate, unrecoverable deletion, skipping any recovery
// window. Only AWS Secrets Manager honors it (mapped to
// ForceDeleteWithoutRecovery). Azure Key Vault ignores it — its retention is a
// vault-level property, not a per-delete option, so deletes stay soft (use
// Restore to recover). Providers without the concept ignore it.
type ForceDelete struct{ DeleteOptionMarker }

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
	// (it never overwrites). Provider-specific WriteOptions are interpreted by
	// the adapter and ignored when unrecognized.
	Create(
		ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...WriteOption,
	) (domain.Version, error)
	// Put creates or updates an entry (upsert) and returns the resulting
	// version. Unlike Create it overwrites an existing entry. Provider-specific
	// WriteOptions are interpreted by the adapter and ignored when unrecognized.
	Put(
		ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...WriteOption,
	) (domain.Version, error)
	// Delete removes an entry. Provider-specific DeleteOptions are interpreted
	// by the adapter and ignored when unrecognized.
	Delete(ctx context.Context, name string, opts ...DeleteOption) error
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
