package secret

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
	Reverse    bool // Reverse chronological order
}

// LogEntry represents a single version entry.
type LogEntry struct {
	VersionID    string
	VersionStage []string
	Value        string
	CreatedDate  *time.Time
	IsCurrent    bool
	Error        error // Error from fetching value, if any
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

// Execute runs the log use case.
//
// It fetches the version history (newest first) via the provider, caps it to
// MaxResults, optionally reverses it, applies the date filters, then retrieves
// each surviving version's value. A per-version fetch failure is recorded on
// the entry's Error field rather than aborting the whole listing.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	versions, err := u.Reader.History(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret versions: %w", err)
	}

	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// The complete history is newest first, so its last element is the very
	// first version that ever existed. Remember it before truncation so we can
	// tell whether the oldest shown version is genuinely the initial one.
	initialVersion := versions[len(versions)-1].ID

	// Apply date filters BEFORE the count limit: -n must return up to N versions
	// that match --since/--until, not N newest-then-filtered to fewer (#351).
	if input.Since != nil || input.Until != nil {
		kept := versions[:0]

		for _, v := range versions {
			if v.Created == nil {
				continue
			}

			if input.Since != nil && v.Created.Before(*input.Since) {
				continue
			}

			if input.Until != nil && v.Created.After(*input.Until) {
				continue
			}

			kept = append(kept, v)
		}

		versions = kept
	}

	// History is newest first; MaxResults caps the number of versions shown.
	if input.MaxResults > 0 && len(versions) > int(input.MaxResults) {
		versions = versions[:input.MaxResults]
	}

	// History yields newest first (default). Reverse to oldest first on request.
	if input.Reverse {
		slices.Reverse(versions)
	}

	entries := make([]LogEntry, 0, len(versions))

	for _, v := range versions {
		value, fetchErr := u.getValue(ctx, input.Name, v.ID)

		entries = append(entries, LogEntry{
			VersionID:    v.ID,
			VersionStage: stages(v.Label),
			Value:        value,
			CreatedDate:  v.Created,
			// The adapter picks the version's Label by priority, so AWSCURRENT
			// always surfaces whenever the version carries it (even alongside a
			// custom label). This equality is therefore a correct membership
			// test, not a fragile "first stage" check (#317).
			IsCurrent: v.Label == "AWSCURRENT",
			Error:     fetchErr,
		})
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
		InitialIncluded: slices.ContainsFunc(entries, func(e LogEntry) bool {
			return e.VersionID == initialVersion
		}),
	}, nil
}

// getValue fetches the value for a specific version id, tolerating fetch
// failures by returning them as an error (the caller records it per-entry).
func (u *LogUseCase) getValue(ctx context.Context, name, versionID string) (string, error) {
	if versionID == "" {
		return "", nil
	}

	ref, err := u.Reader.Resolve(ctx, name, "#"+versionID)
	if err != nil {
		return "", err
	}

	entry, err := u.Reader.Get(ctx, name, ref)
	if err != nil {
		return "", err
	}

	return entry.Value, nil
}
