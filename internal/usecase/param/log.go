package param

import (
	"context"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
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
	Version      int64
	Type         domain.ValueType
	Value        string
	LastModified *time.Time
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
// It fetches the version history (newest first) via the provider, applies the
// date filters, then retrieves each surviving version's value and type. The
// provider's History exposes only version metadata (id/created), so values are
// fetched per version via Resolve+Get. A per-version fetch failure is recorded
// on the entry's Error field rather than aborting the whole listing.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	versions, err := u.Reader.History(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// The complete history is newest first, so its last element is the very
	// first version that ever existed. Remember it before truncation so we can
	// tell whether the oldest shown version is genuinely the initial one.
	initialVersion := parseVersion(versions[len(versions)-1].ID)

	// The current version is the highest version number over the full history.
	maxVersion := lo.MaxBy(versions, func(a, b domain.Version) bool {
		return parseVersion(a.ID) > parseVersion(b.ID)
	})
	maxVersionNum := parseVersion(maxVersion.ID)

	// Apply date filters BEFORE the count limit: -n must return up to N versions
	// that match --since/--until, not N newest-then-filtered to fewer (#351).
	filtered := lo.Filter(versions, func(v domain.Version, _ int) bool {
		if input.Since == nil && input.Until == nil {
			return true
		}

		// Skip versions without a timestamp when date filters are applied.
		if v.Created == nil {
			return false
		}

		if input.Since != nil && v.Created.Before(*input.Since) {
			return false
		}

		if input.Until != nil && v.Created.After(*input.Until) {
			return false
		}

		return true
	})

	// Then cap the (newest-first) filtered set to MaxResults.
	if input.MaxResults > 0 && len(filtered) > int(input.MaxResults) {
		filtered = filtered[:input.MaxResults]
	}

	entries := make([]LogEntry, 0, len(filtered))

	for _, v := range filtered {
		entry, fetchErr := u.getVersion(ctx, input.Name, v)

		logEntry := LogEntry{
			Version:      parseVersion(v.ID),
			LastModified: v.Created,
			IsCurrent:    parseVersion(v.ID) == maxVersionNum,
			Error:        fetchErr,
		}
		// Record a per-version fetch failure on the entry rather than aborting
		// the whole listing; entry is nil when fetchErr is non-nil.
		if fetchErr == nil {
			logEntry.Type = entry.Type
			logEntry.Value = entry.Value
		}

		entries = append(entries, logEntry)
	}

	// History yields newest first (default). Reverse to oldest first on request.
	if input.Reverse {
		slices.Reverse(entries)
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
		InitialIncluded: slices.ContainsFunc(entries, func(e LogEntry) bool {
			return e.Version == initialVersion
		}),
	}, nil
}

// getVersion fetches the value and type for a specific version.
func (u *LogUseCase) getVersion(ctx context.Context, name string, v domain.Version) (*domain.Entry, error) {
	ref, err := u.Reader.Resolve(ctx, name, "#"+v.ID)
	if err != nil {
		return nil, err
	}

	return u.Reader.Get(ctx, name, ref)
}
