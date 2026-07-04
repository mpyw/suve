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

// Execute runs the log use case.
//
// It fetches the version history (newest first) via the provider, applies the
// date filters, then retrieves each surviving version's value and type. The
// provider's History exposes only version metadata (id/created), so values are
// fetched per version via Resolve+Get.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	versions, err := u.Reader.History(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// History is newest first; MaxResults caps the number of versions shown.
	if input.MaxResults > 0 && len(versions) > int(input.MaxResults) {
		versions = versions[:input.MaxResults]
	}

	// The current version is the highest version number in the working set.
	maxVersion := lo.MaxBy(versions, func(a, b domain.Version) bool {
		return parseVersion(a.ID) > parseVersion(b.ID)
	})
	maxVersionNum := parseVersion(maxVersion.ID)

	// Apply date filters against each version's creation time.
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

	entries := make([]LogEntry, 0, len(filtered))

	for _, v := range filtered {
		entry, err := u.getVersion(ctx, input.Name, v)
		if err != nil {
			return nil, err
		}

		entries = append(entries, LogEntry{
			Version:      parseVersion(v.ID),
			Type:         entry.Type,
			Value:        entry.Value,
			LastModified: v.Created,
			IsCurrent:    parseVersion(v.ID) == maxVersionNum,
		})
	}

	// History yields newest first (default). Reverse to oldest first on request.
	if input.Reverse {
		slices.Reverse(entries)
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
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
