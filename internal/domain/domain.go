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

// Version identifies one version of an entry. Label is optional and may be
// empty for providers without human-facing version labels.
type Version struct {
	// ID is the provider-internal version identifier.
	ID string
	// Label is an optional human-facing label (empty when unsupported).
	Label string
	// Created is the version creation time, if known.
	Created *time.Time
}

// Tag is a single key/value label attached to an entry.
type Tag struct{ Key, Value string }

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
}
