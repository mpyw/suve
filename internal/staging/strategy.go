package staging

import (
	"context"
	"time"
)

// ApplyStrategy defines service-specific apply operations.
type ApplyStrategy interface {
	ServiceStrategy

	// Apply applies a staged entry operation to the remote store.
	// Handles OperationCreate, OperationUpdate, and OperationDelete based on entry.Operation.
	Apply(ctx context.Context, name string, entry Entry) error

	// ApplyTags applies staged tag changes to the remote store.
	ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error

	// FetchLastModified returns the last modified time of the resource in the remote store.
	// It returns a *ResourceNotFoundError when the resource does not exist, so
	// callers can distinguish "missing" from "exists but has no modification
	// time" (the latter returns a zero time with a nil error). Providers that
	// disable conflict detection may always return a zero time with a nil error.
	FetchLastModified(ctx context.Context, name string) (time.Time, error)
}

// DiffStrategy defines service-specific diff/fetch operations.
type DiffStrategy interface {
	ServiceStrategy

	// FetchCurrent fetches the current value from the remote store for diffing.
	FetchCurrent(ctx context.Context, name string) (*FetchResult, error)

	// FetchCurrentTags fetches the current tags from the remote store for showing in diff output.
	// Returns nil map if the resource doesn't exist or has no tags.
	FetchCurrentTags(ctx context.Context, name string) (map[string]string, error)
}

// EditStrategy defines service-specific edit operations.
type EditStrategy interface {
	Parser

	// FetchCurrentValue fetches the current value from the remote store for editing.
	// Returns the value and last modified time for conflict detection.
	FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error)
}

// ResetStrategy defines service-specific reset operations.
type ResetStrategy interface {
	Parser

	// FetchVersion fetches the value for a specific version.
	// Returns the value and a version label for display.
	FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error)

	// FetchCurrentValue fetches the current value from the remote store for auto-skip detection.
	// Uses same signature as EditStrategy for implementation reuse.
	FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error)
}

// DeleteStrategy defines service-specific delete staging operations.
type DeleteStrategy interface {
	ServiceStrategy

	// FetchLastModified returns the last modified time of the resource in the remote store.
	// Used for existence and conflict detection when applying delete operations.
	// It returns a *ResourceNotFoundError when the resource does not exist, so
	// callers can distinguish "missing" from "exists but has no modification
	// time" (the latter returns a zero time with a nil error).
	FetchLastModified(ctx context.Context, name string) (time.Time, error)
}

// FullStrategy combines all service-specific strategy interfaces.
// This enables unified stage commands that work with any provider service.
type FullStrategy interface {
	ApplyStrategy
	DiffStrategy
	EditStrategy
	ResetStrategy
}

// StrategyFactory creates a FullStrategy for a given context.
// Used to defer provider client initialization until command execution.
type StrategyFactory func(ctx context.Context) (FullStrategy, error)

// ApplyStrategyResolver resolves the ApplyStrategy for a given namespace. It
// mirrors the per-namespace resolution the apply path uses so a namespaced
// provider (Azure App Configuration) probes each entry against the remote state
// of its OWN namespace rather than the default one.
type ApplyStrategyResolver func(namespace string) (ApplyStrategy, error)
