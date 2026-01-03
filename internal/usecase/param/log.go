package param

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
)

// LogClient is the interface for the log use case.
type LogClient interface {
	paramapi.GetParameterHistoryAPI
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
	Version      int64
	Type         paramapi.ParameterType
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
	Client LogClient
}

// Execute runs the log use case.
func (u *LogUseCase) Execute(ctx context.Context, input LogInput) (*LogOutput, error) {
	result, err := u.Client.GetParameterHistory(ctx, &paramapi.GetParameterHistoryInput{
		Name:           lo.ToPtr(input.Name),
		WithDecryption: lo.ToPtr(true),
		MaxResults:     lo.ToPtr(input.MaxResults),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	params := result.Parameters
	if len(params) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// Find max version
	var maxVersion int64
	for _, p := range params {
		if p.Version > maxVersion {
			maxVersion = p.Version
		}
	}

	// Convert to entries and apply date filters
	var entries []LogEntry
	for _, history := range params {
		// Apply date filters (skip entries without LastModifiedDate when filters are applied)
		if input.Since != nil || input.Until != nil {
			if history.LastModifiedDate == nil {
				continue
			}
			if input.Since != nil && history.LastModifiedDate.Before(*input.Since) {
				continue
			}
			if input.Until != nil && history.LastModifiedDate.After(*input.Until) {
				continue
			}
		}

		entry := LogEntry{
			Version:      history.Version,
			Type:         history.Type,
			Value:        lo.FromPtr(history.Value),
			LastModified: history.LastModifiedDate,
			IsCurrent:    history.Version == maxVersion,
		}
		entries = append(entries, entry)
	}

	// AWS returns oldest first; reverse to show newest first (unless --reverse)
	if !input.Reverse {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
	}, nil
}
