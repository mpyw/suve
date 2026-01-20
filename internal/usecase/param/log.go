package param

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// LogClient is the interface for the log use case.
//
//nolint:iface // Intentionally aliases ParameterReader for type clarity in LogUseCase.
type LogClient interface {
	provider.ParameterReader
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
	Version   string
	Type      string // Parameter type (e.g., "String", "SecureString")
	Value     string
	UpdatedAt *time.Time
	IsCurrent bool
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
	history, err := u.Client.GetParameterHistory(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	params := history.Parameters
	if len(params) == 0 {
		return &LogOutput{Name: input.Name}, nil
	}

	// Find max version for IsCurrent flag
	maxVersion := findMaxVersion(params)

	// Apply date filters
	filtered := filterByDate(params, input.Since, input.Until)

	// Convert to entries
	entries := make([]LogEntry, len(filtered))
	for i, p := range filtered {
		entry := LogEntry{
			Version:   p.Version,
			Value:     p.Value,
			UpdatedAt: p.UpdatedAt,
			IsCurrent: p.Version == maxVersion,
		}

		// Extract Type from AWS metadata if available
		if meta := p.AWSMeta(); meta != nil {
			entry.Type = meta.Type
		}

		entries[i] = entry
	}

	// AWS returns oldest first; reverse to show newest first (unless --reverse)
	if !input.Reverse {
		slices.Reverse(entries)
	}

	// Apply MaxResults limit after sorting
	if input.MaxResults > 0 && len(entries) > int(input.MaxResults) {
		entries = entries[:input.MaxResults]
	}

	return &LogOutput{
		Name:    input.Name,
		Entries: entries,
	}, nil
}

// findMaxVersion returns the maximum version string from the parameters.
func findMaxVersion(params []*model.Parameter) string {
	maxVersion := ""
	maxVersionNum := int64(-1)

	for _, p := range params {
		if v, err := strconv.ParseInt(p.Version, 10, 64); err == nil {
			if v > maxVersionNum {
				maxVersionNum = v
				maxVersion = p.Version
			}
		}
	}

	return maxVersion
}

// filterByDate filters parameters by modification date range.
func filterByDate(params []*model.Parameter, since, until *time.Time) []*model.Parameter {
	if since == nil && until == nil {
		return params
	}

	filtered := make([]*model.Parameter, 0, len(params))

	for _, p := range params {
		// Skip entries without LastModified when date filters are applied
		if p.UpdatedAt == nil {
			continue
		}

		if since != nil && p.UpdatedAt.Before(*since) {
			continue
		}

		if until != nil && p.UpdatedAt.After(*until) {
			continue
		}

		filtered = append(filtered, p)
	}

	return filtered
}
