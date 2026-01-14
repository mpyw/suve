package param

import (
	"context"
	"fmt"
	"slices"
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

	// Find max version using lo.MaxBy
	maxVersion := lo.MaxBy(params, func(a, b paramapi.ParameterHistory) bool {
		return a.Version > b.Version
	}).Version

	// Apply date filters using lo.Filter
	filtered := lo.Filter(params, func(h paramapi.ParameterHistory, _ int) bool {
		// Skip entries without LastModifiedDate when date filters are applied
		if input.Since != nil || input.Until != nil {
			if h.LastModifiedDate == nil {
				return false
			}

			if input.Since != nil && h.LastModifiedDate.Before(*input.Since) {
				return false
			}

			if input.Until != nil && h.LastModifiedDate.After(*input.Until) {
				return false
			}
		}

		return true
	})

	// Convert to entries using lo.Map
	entries := lo.Map(filtered, func(h paramapi.ParameterHistory, _ int) LogEntry {
		return LogEntry{
			Version:      h.Version,
			Type:         h.Type,
			Value:        lo.FromPtr(h.Value),
			LastModified: h.LastModifiedDate,
			IsCurrent:    h.Version == maxVersion,
		}
	})

	// AWS returns oldest first; reverse to show newest first (unless --reverse)
	if !input.Reverse {
		slices.Reverse(entries)
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
	}, nil
}
