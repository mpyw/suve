package gcloud

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/mpyw/suve/internal/provider"
)

// LogInput holds input for the log use case.
type LogInput struct {
	Name       string
	MaxResults int32
	Since      *time.Time
	Until      *time.Time
	Reverse    bool
}

// LogEntry represents a single version entry.
type LogEntry struct {
	Version     string
	State       string // enabled/disabled/destroyed, may be ""
	Value       string
	CreatedDate *time.Time
	Error       error // Error from fetching value, if any (e.g. disabled/destroyed versions)
}

// LogOutput holds the result of the log use case.
type LogOutput struct {
	Name    string
	Entries []LogEntry
}

// LogUseCase executes log operations.
type LogUseCase struct {
	Reader provider.Reader
}

// Execute runs the log use case: fetch the version history (newest first), cap
// to MaxResults, optionally reverse, apply date filters, then retrieve each
// surviving version's value. A per-version fetch failure (common for disabled
// or destroyed versions, whose values are inaccessible) is recorded on the
// entry's Error field rather than aborting the whole listing.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	versions, err := u.Reader.History(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret versions: %w", err)
	}

	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	if input.MaxResults > 0 && len(versions) > int(input.MaxResults) {
		versions = versions[:input.MaxResults]
	}

	if input.Reverse {
		slices.Reverse(versions)
	}

	entries := make([]LogEntry, 0, len(versions))

	for _, v := range versions {
		if input.Since != nil || input.Until != nil {
			if v.Created == nil {
				continue
			}

			if input.Since != nil && v.Created.Before(*input.Since) {
				continue
			}

			if input.Until != nil && v.Created.After(*input.Until) {
				continue
			}
		}

		value, fetchErr := u.getValue(ctx, input.Name, v.ID)

		entries = append(entries, LogEntry{
			Version:     v.ID,
			State:       v.Label,
			Value:       value,
			CreatedDate: v.Created,
			Error:       fetchErr,
		})
	}

	return &LogOutput{Name: input.Name, Entries: entries}, nil
}

// getValue fetches the value for a specific version number, tolerating fetch
// failures by returning them as an error (the caller records it per-entry).
func (u *LogUseCase) getValue(ctx context.Context, name, version string) (string, error) {
	if version == "" {
		return "", nil
	}

	ref, err := u.Reader.Resolve(ctx, name, "#"+version)
	if err != nil {
		return "", err
	}

	entry, err := u.Reader.Get(ctx, name, ref)
	if err != nil {
		return "", err
	}

	return entry.Value, nil
}
