package azure

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
	State       string // enabled/disabled, may be ""
	Value       string
	CreatedDate *time.Time
	Error       error // Error from fetching value, if any (e.g. disabled versions)
}

// LogOutput holds the result of the log use case.
type LogOutput struct {
	Name    string
	Entries []LogEntry
	// InitialIncluded reports whether the oldest entry in Entries is the very
	// first version that ever existed. It is false when the window was cut by
	// --number or a date filter, so the oldest shown version is not a creation.
	InitialIncluded bool
}

// LogUseCase executes log operations.
type LogUseCase struct {
	Reader provider.Reader
}

// Execute runs the log use case: fetch the version history (newest first), cap
// to MaxResults, optionally reverse, apply date filters, then retrieve each
// surviving version's value.
//
// Providers without version history (Azure App Configuration) return an error
// from History; that error is propagated here so the generic log command
// surfaces it cleanly instead of crashing.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	versions, err := u.Reader.History(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version history: %w", err)
	}

	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// The complete history is newest first, so its last element is the very
	// first version that ever existed. Remember it before truncation so we can
	// tell whether the oldest shown version is genuinely the initial one.
	initialVersion := versions[len(versions)-1].ID

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

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
		InitialIncluded: slices.ContainsFunc(entries, func(e LogEntry) bool {
			return e.Version == initialVersion
		}),
	}, nil
}

// getValue fetches the value for a specific version id, tolerating fetch
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
