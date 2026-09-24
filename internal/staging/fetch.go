package staging

import (
	"time"
)

// FetchResult holds the result of fetching a value from the remote store.
type FetchResult struct {
	// Value is the current remote value.
	Value string
	// Identifier is a display string for the version (e.g., "#3" for AWS Parameter Store, "#abc123" for Secrets Manager).
	Identifier string
	// Secret reports whether the fetched value is secret material (a Secrets
	// Manager/Key Vault/Secret Manager value, or a SecureString param), so a
	// staged-diff consumer masks it. A SecureString param is a secret on the
	// value-type axis even though it lives on the param service (#677).
	Secret bool
}

// EditFetchResult holds the result of fetching a value for editing.
type EditFetchResult struct {
	// Value is the current remote value.
	Value string
	// LastModified is the last modification time of the resource.
	// Used for conflict detection when applying staged changes.
	LastModified time.Time
}
