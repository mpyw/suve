// Package staging provides staging functionality for AWS parameter and secret changes.
package staging

import (
	"context"
	"time"
)

// ServiceStrategy defines the common interface for service-specific operations.
// This enables Strategy Pattern to consolidate duplicate code across SSM Parameter Store and Secrets Manager commands.
type ServiceStrategy interface {
	// Service returns the service type (ServiceParam or ServiceSecret).
	Service() Service

	// ServiceName returns the user-friendly service name (e.g., "SSM Parameter Store", "Secrets Manager").
	ServiceName() string

	// ItemName returns the item name for messages (e.g., "parameter", "secret").
	ItemName() string

	// HasDeleteOptions returns true if delete options should be displayed.
	HasDeleteOptions() bool
}

// Parser provides name/spec parsing without AWS access.
// Use this interface when only parsing is needed (e.g., status, add commands).
type Parser interface {
	ServiceStrategy

	// ParseName parses and validates a name, returning only the base name without version specifiers.
	// Returns an error if version specifiers are present.
	ParseName(input string) (string, error)

	// ParseSpec parses a version spec string.
	// Returns the base name and whether a version/shift was specified.
	ParseSpec(input string) (name string, hasVersion bool, err error)
}

// ParserFactory creates a Parser without AWS client.
type ParserFactory func() Parser

// ApplyStrategy defines service-specific apply operations.
type ApplyStrategy interface {
	ServiceStrategy

	// Apply applies a staged operation to AWS.
	// Handles OperationCreate, OperationUpdate, and OperationDelete based on entry.Operation.
	Apply(ctx context.Context, name string, entry Entry) error

	// FetchLastModified returns the last modified time of the resource in AWS.
	// Returns zero time if the resource doesn't exist (for create operations).
	FetchLastModified(ctx context.Context, name string) (time.Time, error)
}

// FetchResult holds the result of fetching a value from AWS.
type FetchResult struct {
	// Value is the current value in AWS.
	Value string
	// Identifier is a display string for the version (e.g., "#3" for SSM Parameter Store, "#abc123" for Secrets Manager).
	Identifier string
}

// EditFetchResult holds the result of fetching a value for editing.
type EditFetchResult struct {
	// Value is the current value in AWS.
	Value string
	// LastModified is the last modification time of the resource.
	// Used for conflict detection when applying staged changes.
	LastModified time.Time
}

// DiffStrategy defines service-specific diff/fetch operations.
type DiffStrategy interface {
	ServiceStrategy

	// FetchCurrent fetches the current value from AWS for diffing.
	FetchCurrent(ctx context.Context, name string) (*FetchResult, error)
}

// EditStrategy defines service-specific edit operations.
type EditStrategy interface {
	Parser

	// FetchCurrentValue fetches the current value from AWS for editing.
	// Returns the value and last modified time for conflict detection.
	FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error)
}

// ResetStrategy defines service-specific reset operations.
type ResetStrategy interface {
	Parser

	// FetchVersion fetches the value for a specific version.
	// Returns the value and a version label for display.
	FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error)
}

// FullStrategy combines all service-specific strategy interfaces.
// This enables unified stage commands that work with either SSM Parameter Store or Secrets Manager.
type FullStrategy interface {
	ApplyStrategy
	DiffStrategy
	EditStrategy
	ResetStrategy
}

// StrategyFactory creates a FullStrategy for a given context.
// Used to defer AWS client initialization until command execution.
type StrategyFactory func(ctx context.Context) (FullStrategy, error)
