package secret

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
)

// LogClient is the interface for the log use case.
type LogClient interface {
	secretapi.ListSecretVersionIdsAPI
	secretapi.GetSecretValueAPI
}

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
	result, err := u.Client.ListSecretVersionIds(ctx, &secretapi.ListSecretVersionIdsInput{
		SecretId:   lo.ToPtr(input.Name),
		MaxResults: lo.ToPtr(input.MaxResults),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secret versions: %w", err)
	}

	versions := result.Versions
	if len(versions) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// Sort by creation date (newest first by default)
	slices.SortFunc(versions, func(a, b secretapi.SecretVersionsListEntry) int {
		if a.CreatedDate == nil || b.CreatedDate == nil {
			return 0
		}
		if a.CreatedDate.After(*b.CreatedDate) {
			return -1
		}
		if a.CreatedDate.Before(*b.CreatedDate) {
			return 1
		}
		return 0
	})

	// Reverse if requested
	if input.Reverse {
		slices.Reverse(versions)
	}

	// Build entries
	var entries []LogEntry
	for _, v := range versions {
		// Apply date filters (skip entries without CreatedDate when filters are applied)
		if input.Since != nil || input.Until != nil {
			if v.CreatedDate == nil {
				continue
			}
			if input.Since != nil && v.CreatedDate.Before(*input.Since) {
				continue
			}
			if input.Until != nil && v.CreatedDate.After(*input.Until) {
				continue
			}
		}

		// Fetch the value for this version
		var value string
		var fetchErr error
		if v.VersionId != nil {
			secretOut, err := u.Client.GetSecretValue(ctx, &secretapi.GetSecretValueInput{
				SecretId:  lo.ToPtr(input.Name),
				VersionId: v.VersionId,
			})
			if err != nil {
				fetchErr = err
			} else {
				value = lo.FromPtr(secretOut.SecretString)
			}
		}

		// Check if this is the current version (has AWSCURRENT stage)
		isCurrent := slices.Contains(v.VersionStages, "AWSCURRENT")

		entries = append(entries, LogEntry{
			VersionID:    lo.FromPtr(v.VersionId),
			VersionStage: v.VersionStages,
			Value:        value,
			CreatedDate:  v.CreatedDate,
			IsCurrent:    isCurrent,
			Error:        fetchErr,
		})
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
	}, nil
}
