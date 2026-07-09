// Package domain defines provider-neutral value types shared across every
// storage backend (AWS SSM, AWS Secrets Manager, and future providers).
//
// It has ZERO knowledge of any cloud SDK: no ARNs, no staging labels, no
// provider metadata bags. Types here carry only cross-provider essentials so
// that the usecase and CLI layers can be written once against a single shape.
package domain

import "time"

// ValueType classifies the nature of an entry's value in a provider-neutral
// way. Providers map their own native types onto these values.
type ValueType string

const (
	// ValueTypePlaintext is a plain, non-secret value (AWS String).
	ValueTypePlaintext ValueType = "plaintext" // AWS String, plain values
	// ValueTypeSecret is an encrypted/secret value (AWS SecureString, Secrets Manager).
	ValueTypeSecret ValueType = "secret" // AWS SecureString, Secrets Manager
	// ValueTypeList is a list of values (AWS StringList).
	ValueTypeList ValueType = "list" // AWS StringList
)

// Version identifies one version of an entry.
//
// It carries two independent, provider-specific axes that must NOT be
// conflated (#419):
//
//   - StagingLabels are movable pointers naming "which version is current"
//     (AWS Secrets Manager: AWSCURRENT / AWSPENDING / AWSPREVIOUS). Rotation
//     moves a label from one version to another; a version may carry several
//     labels or none (unlabeled versions are passively retained history, still
//     readable by ID). Multi-valued.
//   - State is a per-version enable/disable switch for reading that specific
//     version's value (Google Cloud + Azure Key Vault). It is single-valued and
//     orthogonal to "which version is latest".
//
// Providers that have neither concept (AWS SSM, Azure App Configuration) leave
// both empty.
type Version struct {
	// ID is the provider-internal version identifier.
	ID string
	// State is the per-version lifecycle state: "enabled" / "disabled" /
	// "destroyed", or "" when the provider has no such concept. It is a
	// per-version switch controlling whether that version's value can be read
	// (Google Cloud + Azure Key Vault); it is orthogonal to which version is
	// latest. Empty for AWS Secrets Manager, AWS SSM and Azure App Config.
	State string
	// StagingLabels are the AWS Secrets Manager staging labels for this version
	// (all of them, e.g. AWSCURRENT / AWSPREVIOUS). A staging label is a movable
	// pointer naming "which version is current", not a per-version state.
	// nil/empty for every other provider.
	StagingLabels []string
	// Created is the version creation time, if known.
	Created *time.Time
	// Tags are the labels attached to THIS version. Only Azure Key Vault scopes
	// tags per version (each version has its own); every other provider keeps
	// tags at the resource level (see Entry.Tags) and leaves this nil.
	Tags []Tag
}

// Tag is a single key/value label attached to an entry.
type Tag struct{ Key, Value string }

// Field is a neutral, display-only piece of provider metadata surfaced on a
// read Entry (e.g. an AWS Secrets Manager ARN). It carries a human-facing Label
// and a pre-formatted string Value only: it is deliberately provider-neutral in
// SHAPE (no AWS types, no any) so the usecase/CLI layers can render it without
// knowing which provider produced it. Providers populate Entry.Extra; consumers
// display it verbatim and never interpret it.
type Field struct {
	// Label is the human-facing field name (e.g. "ARN").
	Label string
	// Value is the pre-formatted display value.
	Value string
}

// TagChange describes staged tag mutations (add/update + remove-by-key).
type TagChange struct {
	// Add holds tags to create or update, keyed by tag key.
	Add map[string]string
	// Remove holds tag keys to delete.
	Remove []string
}

// Entry is a provider-neutral retrieved parameter/secret. It carries only
// cross-provider essentials: no ARN, no provider metadata bag.
type Entry struct {
	// Name is the entry's identifier within its provider namespace.
	Name string
	// Value is the entry's current value.
	Value string
	// Type classifies the value in a provider-neutral way.
	Type ValueType
	// Version identifies the retrieved version.
	Version Version
	// Description is an optional human-readable description.
	Description string
	// Tags are the labels attached to the entry.
	Tags []Tag
	// Modified is the last-modified time, if known.
	Modified *time.Time
	// Extra holds provider-populated, display-only metadata (e.g. an AWS
	// Secrets Manager ARN). Its SHAPE is provider-neutral: a list of neutral
	// Field values, never AWS types or an untyped any. Providers may leave it
	// empty when they have no extra metadata to surface.
	Extra []Field
}
