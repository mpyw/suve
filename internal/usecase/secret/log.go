package secret

import (
	"context"
	"fmt"
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
//
// VersionStage and State carry the two independent, provider-specific axes that
// must NOT be conflated (#419): VersionStage holds AWS Secrets Manager staging
// labels (empty for other providers), while State holds the per-version
// lifecycle state (enabled/disabled/destroyed) for Google Cloud + Azure Key
// Vault (empty for AWS). A version never has both.
type LogEntry struct {
	VersionID    string
	VersionStage []string
	State        string
	Value        string
	CreatedDate  *time.Time
	IsCurrent    bool
	// Tags attached to THIS version. Only Azure Key Vault scopes tags per version;
	// empty for providers whose tags live at the resource level.
	Tags  []domain.Tag
	Error error // Error from fetching value, if any
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

	// Determine the current version up front (over the FULL history, before any
	// truncation/reverse): AWS Secrets Manager names it with the AWSCURRENT
	// staging label; Google Cloud and Azure Key Vault have no such label, so the
	// newest version is the current one.
	currentVersion := currentVersionID(versions)

	// Apply date filters BEFORE the count limit: -n must return up to N versions
	// that match --since/--until, not N newest-then-filtered to fewer (#351).
	versions = lo.Filter(versions, func(v domain.Version, _ int) bool {
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

	// History is newest first; MaxResults caps the number of versions shown.
	if input.MaxResults > 0 && len(versions) > int(input.MaxResults) {
		versions = versions[:input.MaxResults]
	}

	// History yields newest first (default). Reverse to oldest first on request.
	if input.Reverse {
		slices.Reverse(versions)
	}

	entries := lo.Map(versions, func(v domain.Version, _ int) LogEntry {
		value, fetchErr := u.getValue(ctx, input.Name, v.ID)

		return LogEntry{
			VersionID:    v.ID,
			VersionStage: stages(v.StagingLabels),
			State:        v.State,
			Value:        value,
			CreatedDate:  v.Created,
			IsCurrent:    v.ID == currentVersion,
			Tags:         v.Tags,
			Error:        fetchErr,
		}
	})

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
		InitialIncluded: slices.ContainsFunc(entries, func(e LogEntry) bool {
			return e.VersionID == initialVersion
		}),
	}, nil
}

// currentVersionID returns the id of the current version. AWS Secrets Manager
// marks it with the AWSCURRENT staging label (membership, not position, so it
// stays correct even when the version carries extra custom labels, #317). Google
// Cloud and Azure Key Vault have no staging labels, so the newest version (first
// in the newest-first history) is the current one. versions must be non-empty.
func currentVersionID(versions []domain.Version) string {
	for _, v := range versions {
		if slices.Contains(v.StagingLabels, "AWSCURRENT") {
			return v.ID
		}
	}

	return versions[0].ID
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
