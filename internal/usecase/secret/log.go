package secret

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/mpyw/suve/internal/model"
)

// LogClient is the interface for the log use case.
type LogClient = VersionResolverClient

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
}

// LogUseCase executes log operations.
type LogUseCase struct {
	Client LogClient
}

// Execute runs the log use case.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	// List all versions
	versions, err := u.Client.GetSecretVersions(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret versions: %w", err)
	}

	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// Apply MaxResults limit if specified
	if input.MaxResults > 0 && int(input.MaxResults) < len(versions) {
		versions = versions[:input.MaxResults]
	}

	// Sort by creation date (newest first by default)
	slices.SortFunc(versions, func(a, b *model.SecretVersion) int {
		if a.CreatedAt == nil || b.CreatedAt == nil {
			return 0
		}

		if a.CreatedAt.After(*b.CreatedAt) {
			return -1
		}

		if a.CreatedAt.Before(*b.CreatedAt) {
			return 1
		}

		return 0
	})

	// Reverse if requested
	if input.Reverse {
		slices.Reverse(versions)
	}

	// Build entries
	entries := make([]LogEntry, 0, len(versions))

	for _, v := range versions {
		// Apply date filters (skip entries without CreatedAt when filters are applied)
		if input.Since != nil || input.Until != nil {
			if v.CreatedAt == nil {
				continue
			}

			if input.Since != nil && v.CreatedAt.Before(*input.Since) {
				continue
			}

			if input.Until != nil && v.CreatedAt.After(*input.Until) {
				continue
			}
		}

		// Fetch the value for this version
		var (
			value    string
			fetchErr error
		)

		if v.Version != "" {
			secret, err := u.Client.GetSecret(ctx, input.Name, v.Version, "")
			if err != nil {
				fetchErr = err
			} else {
				value = secret.Value
			}
		}

		// Check if this is the current version (has AWSCURRENT stage)
		var versionStages []string

		isCurrent := false

		if meta, ok := v.Metadata.(model.AWSSecretVersionMeta); ok {
			versionStages = meta.VersionStages
			isCurrent = slices.Contains(meta.VersionStages, "AWSCURRENT")
		}

		entries = append(entries, LogEntry{
			VersionID:    v.Version,
			VersionStage: versionStages,
			Value:        value,
			CreatedDate:  v.CreatedAt,
			IsCurrent:    isCurrent,
			Error:        fetchErr,
		})
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
	}, nil
}
