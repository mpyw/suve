// Package stage provides staging functionality for AWS parameter and secret changes.
package stage

import (
	"context"
)

// ServiceStrategy defines the common interface for service-specific operations.
// This enables Strategy Pattern to consolidate duplicate code across SSM and SM commands.
type ServiceStrategy interface {
	// Service returns the service type (ServiceSSM or ServiceSM).
	Service() Service

	// ServiceName returns the user-friendly service name (e.g., "SSM", "SM").
	ServiceName() string

	// ItemName returns the item name for messages (e.g., "parameter", "secret").
	ItemName() string

	// HasDeleteOptions returns true if delete options should be displayed.
	HasDeleteOptions() bool
}

// PushStrategy defines service-specific push operations.
type PushStrategy interface {
	ServiceStrategy

	// PushSet applies a set operation to AWS.
	PushSet(ctx context.Context, name, value string) error

	// PushDelete applies a delete operation to AWS.
	PushDelete(ctx context.Context, name string, entry Entry) error
}

// FetchResult holds the result of fetching a value from AWS.
type FetchResult struct {
	// Value is the current value in AWS.
	Value string
	// VersionLabel is a display string for the version (e.g., "#3" for SSM, "#abc123" for SM).
	VersionLabel string
}

// DiffStrategy defines service-specific diff/fetch operations.
type DiffStrategy interface {
	ServiceStrategy

	// FetchCurrent fetches the current value from AWS for diffing.
	FetchCurrent(ctx context.Context, name string) (*FetchResult, error)
}

// EditStrategy defines service-specific edit operations.
type EditStrategy interface {
	ServiceStrategy

	// ParseName parses and validates a name, returning only the base name without version specifiers.
	// Returns an error if version specifiers are present.
	ParseName(input string) (string, error)

	// FetchCurrentValue fetches the current value from AWS for editing.
	FetchCurrentValue(ctx context.Context, name string) (string, error)
}

// ResetStrategy defines service-specific reset operations.
type ResetStrategy interface {
	ServiceStrategy

	// ParseSpec parses a version spec string.
	// Returns the base name and whether a version/shift was specified.
	ParseSpec(input string) (name string, hasVersion bool, err error)

	// FetchVersion fetches the value for a specific version.
	// Returns the value and a version label for display.
	FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error)
}
